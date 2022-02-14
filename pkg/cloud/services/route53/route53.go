package route53

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/giantswarm/microerror"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	appNameLabelKey = "app.kubernetes.io/name"

	ingressAppLabel     = "nginx-ingress-controller"
	ingressAppNamespace = "kube-system"
	ttl                 = 300

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

	if err := s.deleteClusterRecords(ctx, hostedZoneID); err != nil {
		return microerror.Mask(err)
	}

	// Then delete delegation record in base hosted zone
	err = s.changeClusterNSDelegation(ctx, hostedZoneID, actionDelete)
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
	hostedZoneID, err := s.describeClusterHostedZone(ctx)
	if IsHostedZoneNotFound(err) {
		hostedZoneID, err = s.createClusterHostedZone(ctx)
		if err != nil {
			return microerror.Mask(err)
		}
		s.scope.Info(fmt.Sprintf("Created new hosted zone for cluster %s", s.scope.Name()))
	} else if err != nil {
		return microerror.Mask(err)
	}

	if err := s.changeClusterNSDelegation(ctx, hostedZoneID, actionUpsert); err != nil {
		return microerror.Mask(err)
	}

	if err := s.changeClusterRecords(ctx, hostedZoneID, actionUpsert); err != nil {
		return microerror.Mask(err)
	}

	if err := s.changeClusterIngressRecords(ctx, hostedZoneID, actionUpsert); err != nil {
		return microerror.Mask(err)
	}

	return nil
}

func (s *Service) buildARecordChange(hostedZoneID, recordName, recordValue, action string) *route53.Change {
	return &route53.Change{
		Action: aws.String(action),
		ResourceRecordSet: &route53.ResourceRecordSet{
			Name: aws.String(fmt.Sprintf("%s.%s", recordName, s.scope.ClusterDomain())),
			Type: aws.String("A"),
			TTL:  aws.Int64(ttl),
			ResourceRecords: []*route53.ResourceRecord{
				{
					Value: aws.String(recordValue),
				},
			},
		},
	}
}

func (s *Service) changeClusterIngressRecords(ctx context.Context, hostedZoneID, action string) error {
	ingressIP, err := s.getIngressIP(ctx)
	if err != nil {
		return microerror.Mask(err)
	} else if ingressIP == "" {
		return nil
	}

	input := &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(hostedZoneID),
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{
				s.buildARecordChange(hostedZoneID, "ingress", ingressIP, action),
				{
					Action: aws.String(action),
					ResourceRecordSet: &route53.ResourceRecordSet{
						Name: aws.String(fmt.Sprintf("*.%s", s.scope.ClusterDomain())),
						Type: aws.String("CNAME"),
						TTL:  aws.Int64(300),
						ResourceRecords: []*route53.ResourceRecord{
							{
								Value: aws.String(fmt.Sprintf("ingress.%s", s.scope.ClusterDomain())),
							},
						},
					},
				},
			},
		},
	}

	if _, err := s.Route53Client.ChangeResourceRecordSetsWithContext(ctx, input); err != nil {
		return wrapRoute53Error(err)
	}
	return nil
}

func (s *Service) changeClusterNSDelegation(ctx context.Context, hostedZoneID, action string) error {
	records, err := s.listClusterNSRecords(ctx, hostedZoneID)
	if err != nil {
		return microerror.Mask(err)
	}

	baseHostedZoneID, err := s.describeBaseHostedZone(ctx)
	if err != nil {
		return microerror.Mask(err)
	}

	input := &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(baseHostedZoneID),
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{
				{
					Action: aws.String(action),
					ResourceRecordSet: &route53.ResourceRecordSet{
						Name:            aws.String(s.scope.ClusterDomain()),
						Type:            aws.String("NS"),
						TTL:             aws.Int64(ttl),
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

func (s *Service) changeClusterRecords(ctx context.Context, hostedZoneID string, action string) error {
	input := &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(hostedZoneID),
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{},
		},
	}

	if s.scope.APIEndpoint() == "" {
		s.scope.Info("API endpoint is not ready yet.")
		return aws.ErrMissingEndpoint
	}

	s.scope.Info(s.scope.APIEndpoint())

	input.ChangeBatch.Changes = append(input.ChangeBatch.Changes,
		s.buildARecordChange(hostedZoneID, "api", s.scope.APIEndpoint(), actionUpsert),
	)

	if s.scope.BastionIP() != "" {
		s.scope.Info(s.scope.BastionIP())

		input.ChangeBatch.Changes = append(input.ChangeBatch.Changes,
			s.buildARecordChange(hostedZoneID, "bastion1", s.scope.BastionIP(), actionUpsert),
		)
	}

	if _, err := s.Route53Client.ChangeResourceRecordSetsWithContext(ctx, input); err != nil {
		return wrapRoute53Error(err)
	}
	return nil
}

func (s *Service) createClusterHostedZone(ctx context.Context) (string, error) {
	now := time.Now()
	input := &route53.CreateHostedZoneInput{
		CallerReference: aws.String(now.UTC().String()),
		Name:            aws.String(fmt.Sprintf("%s.", s.scope.ClusterDomain())),
	}

	if s.scope.ManagementCluster() != "" {
		input.HostedZoneConfig = &route53.HostedZoneConfig{
			Comment: aws.String(fmt.Sprintf("management_cluster: %s", s.scope.ManagementCluster())),
		}
	}
	output, err := s.Route53Client.CreateHostedZoneWithContext(ctx, input)
	if err != nil {
		return "", wrapRoute53Error(err)
	}

	return *output.HostedZone.Id, nil
}

func (s *Service) deleteClusterRecords(ctx context.Context, hostedZoneID string) error {
	input := &route53.ListResourceRecordSetsInput{
		HostedZoneId: aws.String(hostedZoneID),
	}

	output, err := s.Route53Client.ListResourceRecordSetsWithContext(ctx, input)
	if err != nil {
		return wrapRoute53Error(err)
	}

	recordsToDelete := &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(hostedZoneID),
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{},
		},
	}

	for _, recordSet := range output.ResourceRecordSets {
		if strings.TrimSuffix(*recordSet.Name, ".") == s.scope.ClusterDomain() {
			// We cannot delete those entries, they get automatically cleaned up when deleting the hosted zone
			continue
		}

		recordsToDelete.ChangeBatch.Changes = append(recordsToDelete.ChangeBatch.Changes, &route53.Change{
			Action:            aws.String(actionDelete),
			ResourceRecordSet: recordSet,
		})
	}

	if len(recordsToDelete.ChangeBatch.Changes) == 0 {
		// Nothing to delete
		return nil
	}

	_, err = s.Route53Client.ChangeResourceRecordSetsWithContext(ctx, recordsToDelete)
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

func (s *Service) describeClusterHostedZone(ctx context.Context) (string, error) {
	// Search host zone by DNSName
	input := &route53.ListHostedZonesByNameInput{
		DNSName: aws.String(s.scope.ClusterDomain()),
	}
	out, err := s.Route53Client.ListHostedZonesByNameWithContext(ctx, input)
	if err != nil {
		return "", wrapRoute53Error(err)
	}

	if len(out.HostedZones) == 0 {
		return "", microerror.Mask(hostedZoneNotFoundError)
	}

	if *out.HostedZones[0].Name != fmt.Sprintf("%s.", s.scope.ClusterDomain()) {
		return "", microerror.Mask(hostedZoneNotFoundError)
	}

	return *out.HostedZones[0].Id, nil
}

func (s *Service) getIngressIP(ctx context.Context) (string, error) {

	k8sClient, err := s.scope.ClusterK8sClient(ctx)
	if err != nil {
		return "", microerror.Mask(err)
	}

	var icServices corev1.ServiceList

	err = k8sClient.List(ctx, &icServices,
		client.InNamespace(ingressAppNamespace),
		client.MatchingLabels{appNameLabelKey: ingressAppLabel},
	)

	if err != nil {
		return "", microerror.Mask(err)
	}

	for _, icService := range icServices.Items {
		if icService.Spec.Type == corev1.ServiceTypeLoadBalancer {
			if len(icService.Status.LoadBalancer.Ingress) < 1 || icService.Status.LoadBalancer.Ingress[0].IP == "" {
				return "", microerror.Mask(ingressNotReadyError)
			}

			return icService.Status.LoadBalancer.Ingress[0].IP, nil
		}
	}

	// Ingress service is not installed in this cluster.
	return "", nil
}

func (s *Service) listClusterNSRecords(ctx context.Context, hostedZoneID string) ([]*route53.ResourceRecord, error) {
	// First entry is always NS record
	input := &route53.ListResourceRecordSetsInput{
		HostedZoneId: aws.String(hostedZoneID),
		MaxItems:     aws.String("1"),
	}

	output, err := s.Route53Client.ListResourceRecordSetsWithContext(ctx, input)
	if err != nil {
		return nil, wrapRoute53Error(err)
	}
	return output.ResourceRecordSets[0].ResourceRecords, nil
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
