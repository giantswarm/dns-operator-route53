package route53

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/giantswarm/microerror"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	IngressAppPrefix    = "nginx-ingress-controller-app-"
	IngressAppNamespace = "kube-system"
	TTL                 = 300

	actionDelete = "DELETE"
	actionUpsert = "UPSERT"
)

func (s *Service) DeleteRoute53(ctx context.Context) error {
	s.scope.V(2).Info("Deleting hosted DNS zone")
	hostedZoneID, err := s.describeClusterHostedZone(ctx)
	if IsHostedZoneNotFound(err) {
		return nil
	} else if err != nil {
		return microerror.Mask(err)
	}

	// We need to delete all records first before we can delete the hosted zone
	err = s.changeClusterIngressRecords(ctx, actionDelete)
	if IsNotFound(err) {
		// Entry does not exist, fall through
	} else if err != nil {
		return microerror.Mask(err)
	}

	err = s.changeClusterAPIRecords(ctx, actionDelete)
	if IsNotFound(err) {
		// Entry does not exist, fall through
	} else if err != nil {
		return microerror.Mask(err)
	}

	// Then delete delegation record in base hosted zone
	err = s.changeClusterNSDelegation(ctx, actionDelete)
	if IsNotFound(err) {
		// Entry does not exist, fall through
	} else if err != nil {
		return microerror.Mask(err)
	}

	// Finally delete DNS zone for cluster
	err = s.deleteClusterHostedZone(ctx, hostedZoneID)
	if err != nil {
		return microerror.Mask(err)
	}
	s.scope.V(2).Info(fmt.Sprintf("Deleting hosted zone completed successfully for cluster %s", s.scope.Name()))
	return nil
}

func (s *Service) ReconcileRoute53(ctx context.Context) error {
	s.scope.V(2).Info("Reconciling hosted DNS zone")

	// Describe or create.
	_, err := s.describeClusterHostedZone(ctx)
	if IsHostedZoneNotFound(err) {
		err = s.createClusterHostedZone(ctx)
		if err != nil {
			return microerror.Mask(err)
		}
		s.scope.Info(fmt.Sprintf("Created new hosted zone for cluster %s", s.scope.Name()))
	} else if err != nil {
		return microerror.Mask(err)
	}

	if err := s.changeClusterNSDelegation(ctx, actionUpsert); err != nil {
		return microerror.Mask(err)
	}

	if err := s.changeClusterAPIRecords(ctx, actionUpsert); err != nil {
		return microerror.Mask(err)
	}

	if err := s.changeClusterIngressRecords(ctx, actionUpsert); err != nil {
		return microerror.Mask(err)
	}

	return nil
}

func (s *Service) describeClusterHostedZone(ctx context.Context) (string, error) {
	// Search host zone by DNSName
	input := &route53.ListHostedZonesByNameInput{
		DNSName: aws.String(fmt.Sprintf("%s.%s", s.scope.Name(), s.scope.BaseDomain())),
	}
	out, err := s.Route53Client.ListHostedZonesByNameWithContext(ctx, input)
	if err != nil {
		return "", wrapRoute53Error(err)
	}

	if len(out.HostedZones) == 0 {
		return "", microerror.Mask(hostedZoneNotFoundError)
	}

	if *out.HostedZones[0].Name != fmt.Sprintf("%s.%s.", s.scope.Name(), s.scope.BaseDomain()) {
		return "", microerror.Mask(hostedZoneNotFoundError)
	}

	return *out.HostedZones[0].Id, nil
}

func (s *Service) listClusterNSRecords(ctx context.Context) ([]*route53.ResourceRecord, error) {
	hostZoneID, err := s.describeClusterHostedZone(ctx)
	if err != nil {
		return nil, microerror.Mask(err)
	}

	// First entry is always NS record
	input := &route53.ListResourceRecordSetsInput{
		HostedZoneId: aws.String(hostZoneID),
		MaxItems:     aws.String("1"),
	}

	output, err := s.Route53Client.ListResourceRecordSetsWithContext(ctx, input)
	if err != nil {
		return nil, wrapRoute53Error(err)
	}
	return output.ResourceRecordSets[0].ResourceRecords, nil
}

func (s *Service) changeClusterAPIRecords(ctx context.Context, action string) error {
	s.scope.Info(s.scope.APIEndpoint())
	if s.scope.APIEndpoint() == "" {
		s.scope.Info("API endpoint is not ready yet.")
		return aws.ErrMissingEndpoint
	}

	hostZoneID, err := s.describeClusterHostedZone(ctx)
	if err != nil {
		return microerror.Mask(err)
	}

	input := &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(hostZoneID),
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{
				{
					Action: aws.String(action),
					ResourceRecordSet: &route53.ResourceRecordSet{
						Name: aws.String(fmt.Sprintf("api.%s.%s", s.scope.Name(), s.scope.BaseDomain())),
						Type: aws.String("A"),
						TTL:  aws.Int64(300),
						ResourceRecords: []*route53.ResourceRecord{
							{
								Value: aws.String(s.scope.APIEndpoint()),
							},
						},
					},
				},
			},
		},
	}

	_, err = s.Route53Client.ChangeResourceRecordSetsWithContext(ctx, input)
	if err != nil {
		return wrapRoute53Error(err)
	}
	return nil
}

func (s *Service) changeClusterIngressRecords(ctx context.Context, action string) error {
	hostZoneID, err := s.describeClusterHostedZone(ctx)
	if err != nil {
		return microerror.Mask(err)
	}

	var ingressIP string
	if action == actionUpsert { // Avoid looking up IP via k8s client when deleting
		ingressIP, err = s.getIngressIP(ctx)
		if err != nil {
			return microerror.Mask(err)
		} else if ingressIP == "" {
			return nil
		}
	}

	input := &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(hostZoneID),
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{
				{
					Action: aws.String(action),
					ResourceRecordSet: &route53.ResourceRecordSet{
						Name: aws.String(fmt.Sprintf("ingress.%s.%s", s.scope.Name(), s.scope.BaseDomain())),
						Type: aws.String("A"),
						TTL:  aws.Int64(TTL),
						ResourceRecords: []*route53.ResourceRecord{
							{
								Value: aws.String(ingressIP),
							},
						},
					},
				},
				{
					Action: aws.String(action),
					ResourceRecordSet: &route53.ResourceRecordSet{
						Name: aws.String(fmt.Sprintf("*.%s.%s", s.scope.Name(), s.scope.BaseDomain())),
						Type: aws.String("CNAME"),
						TTL:  aws.Int64(300),
						ResourceRecords: []*route53.ResourceRecord{
							{
								Value: aws.String(fmt.Sprintf("ingress.%s.%s", s.scope.Name(), s.scope.BaseDomain())),
							},
						},
					},
				},
			},
		},
	}

	_, err = s.Route53Client.ChangeResourceRecordSetsWithContext(ctx, input)
	if err != nil {
		return wrapRoute53Error(err)
	}
	return nil
}

func (s *Service) describeBaseHostedZone(ctx context.Context) (string, error) {
	input := &route53.ListHostedZonesByNameInput{
		DNSName: aws.String(s.scope.BaseDomain()),
	}
	out, err := s.Route53Client.ListHostedZonesByNameWithContext(ctx, input)
	if err != nil {
		s.scope.Info(err.Error())
		return "", wrapRoute53Error(err)
	}
	if len(out.HostedZones) == 0 {
		return "", microerror.Mask(hostedZoneNotFoundError)
	}

	if *out.HostedZones[0].Name != fmt.Sprintf("%s.", s.scope.BaseDomain()) {
		return "", microerror.Mask(hostedZoneNotFoundError)
	}

	return *out.HostedZones[0].Id, nil
}

func (s *Service) changeClusterNSDelegation(ctx context.Context, action string) error {
	hostZoneID, err := s.describeBaseHostedZone(ctx)
	if err != nil {
		return microerror.Mask(err)
	}

	records, err := s.listClusterNSRecords(ctx)
	if err != nil {
		return microerror.Mask(err)
	}

	input := &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(hostZoneID),
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{
				{
					Action: aws.String(action),
					ResourceRecordSet: &route53.ResourceRecordSet{
						Name:            aws.String(fmt.Sprintf("%s.%s", s.scope.Name(), s.scope.BaseDomain())),
						Type:            aws.String("NS"),
						TTL:             aws.Int64(TTL),
						ResourceRecords: records,
					},
				},
			},
		},
	}

	_, err = s.Route53Client.ChangeResourceRecordSetsWithContext(ctx, input)
	if err != nil {
		return wrapRoute53Error(err)
	}
	return nil
}

func (s *Service) createClusterHostedZone(ctx context.Context) error {
	now := time.Now()
	input := &route53.CreateHostedZoneInput{
		CallerReference: aws.String(now.UTC().String()),
		Name:            aws.String(fmt.Sprintf("%s.%s.", s.scope.Name(), s.scope.BaseDomain())),
	}

	if s.scope.ManagementCluster() != "" {
		input.HostedZoneConfig = &route53.HostedZoneConfig{
			Comment: aws.String(fmt.Sprintf("MC: %s", s.scope.ManagementCluster())),
		}
	}
	_, err := s.Route53Client.CreateHostedZoneWithContext(ctx, input)
	if err != nil {
		return wrapRoute53Error(err)
	}
	return nil
}

func (s *Service) deleteClusterHostedZone(ctx context.Context, hostedZoneID string) error {
	input := &route53.DeleteHostedZoneInput{
		Id: aws.String(hostedZoneID),
	}
	_, err := s.Route53Client.DeleteHostedZoneWithContext(ctx, input)
	if err != nil {
		return wrapRoute53Error(err)
	}
	return nil
}

func (s *Service) getIngressIP(ctx context.Context) (string, error) {
	serviceName := fmt.Sprintf("%s%s", IngressAppPrefix, s.scope.Name())

	o := client.ObjectKey{
		Name:      serviceName,
		Namespace: IngressAppNamespace,
	}

	k8sClient, err := s.scope.ClusterK8sClient(ctx)
	if err != nil {
		return "", microerror.Mask(err)
	}

	var icService corev1.Service
	err = k8sClient.Get(ctx, o, &icService)
	// Ingress service is not installed in this cluster.
	if apierrors.IsNotFound(err) {
		return "", nil
	} else if err != nil {
		return "", microerror.Mask(err)
	}

	if len(icService.Status.LoadBalancer.Ingress) < 1 || icService.Status.LoadBalancer.Ingress[0].IP == "" {
		return "", microerror.Mask(ingressNotReadyError)
	}

	return icService.Status.LoadBalancer.Ingress[0].IP, nil
}
