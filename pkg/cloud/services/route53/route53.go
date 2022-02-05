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
	logger := s.scope.WithValues("clusterDomain", s.scope.ClusterDomain())
	logger.Info("deleting route53 resources")

	hostedZone, err := s.findHostedZoneByDNSName(ctx, s.scope.ClusterDomain())
	if IsHostedZoneNotFound(err) {
		s.scope.Info("hosted zone not found")
		// nothing to do
		return nil
	} else if err != nil {
		return microerror.Mask(err)
	}
	logger = logger.WithValues("hostedZoneID", *hostedZone.Id)

	if *hostedZone.ResourceRecordSetCount != int64(2) {
		logger.Info("deleting all dns records from zone")
		if err := s.deleteClusterRecords(ctx, *hostedZone.Id); err != nil {
			return microerror.Mask(err)
		}
		logger.Info("deleted all dns records from zone")
	} else {
		logger.Info("zone has no records to delete")
	}

	// Then delete delegation record in base hosted zone
	logger.Info("deleting delegation records from base hosted zone")
	err = s.changeClusterNSDelegation(ctx, *hostedZone.Id, actionDelete)
	if IsNotFound(err) {
		// Entry does not exist, fall through
		logger.Info("no delegation entries found in base hosted zone")
	} else if err != nil {
		return microerror.Mask(err)
	} else {
		logger.Info("deleted delegation records from base hosted zone")
	}

	// Finally delete DNS zone for cluster
	logger.Info("deleting hosted zone")
	err = s.deleteClusterHostedZone(ctx, *hostedZone.Id)
	if err != nil {
		return microerror.Mask(err)
	}
	logger.Info("deleted hosted zone for cluster")
	logger.Info("route53 deletion complete")

	return nil
}

func (s *Service) ReconcileRoute53(ctx context.Context) error {
	logger := s.scope.WithValues("clusterDomain", s.scope.ClusterDomain())
	logger.Info("creating or updating route53 resources")

	// Describe or create.
	hostedZone, err := s.findHostedZoneByDNSName(ctx, s.scope.ClusterDomain())
	if IsHostedZoneNotFound(err) {
		logger.Info("existing hosted zone not found, creating new hosted zone")
		hostedZone, err = s.createClusterHostedZone(ctx)
		if err != nil {
			return microerror.Mask(err)
		}
		logger.Info("created new hosted zone for cluster", "hostedZoneID", *hostedZone.Id)
	} else if err != nil {
		return microerror.Mask(err)
	} else {
		logger.Info("found existing hosted zone for cluster", "hostedZoneID", *hostedZone.Id)
	}
	logger = logger.WithValues("hostedZoneID", *hostedZone.Id)

	expectedComment := fmt.Sprintf("management_cluster: %s", s.scope.ManagementCluster())
	if *hostedZone.Config.Comment != expectedComment {
		err = s.updateHostedZoneComment(ctx, *hostedZone.Id, expectedComment)
		if err != nil {
			return microerror.Mask(err)
		}
	}

	logger.Info("creating or updating hosted zone NS records")
	if err := s.changeClusterNSDelegation(ctx, *hostedZone.Id, actionUpsert); err != nil {
		return microerror.Mask(err)
	}
	logger.Info("ensured hosted zone NS records")

	logger.Info("creating or updating cluster DNS records")
	if err := s.upsertClusterRecords(ctx, *hostedZone.Id); err != nil {
		return microerror.Mask(err)
	}
	logger.Info("ensured cluster DNS records")

	logger.Info("creating or updating ingress records")
	if err := s.upsertClusterIngressRecords(ctx, *hostedZone.Id); err != nil {
		return microerror.Mask(err)
	}
	logger.Info("ensured ingress records")

	return nil
}

func (s *Service) updateHostedZoneComment(ctx context.Context, hostedZoneID, comment string) error {
	_, err := s.Route53Client.UpdateHostedZoneCommentWithContext(ctx, &route53.UpdateHostedZoneCommentInput{
		Comment: aws.String(comment),
		Id:      aws.String(hostedZoneID),
	})
	return microerror.Mask(err)
}

func (s *Service) buildARecordChange(recordName, recordValue, action string) *route53.Change {
	return &route53.Change{
		Action: aws.String(action),
		ResourceRecordSet: &route53.ResourceRecordSet{
			Name: aws.String(fmt.Sprintf("%s.%s", recordName, s.scope.ClusterDomain())),
			Type: aws.String("A"),
			TTL:  aws.Int64(TTL),
			ResourceRecords: []*route53.ResourceRecord{
				{
					Value: aws.String(recordValue),
				},
			},
		},
	}
}

func (s *Service) upsertClusterIngressRecords(ctx context.Context, hostedZoneID string) error {
	ingressIP, err := s.getIngressIP(ctx)
	if err != nil {
		return microerror.Mask(err)
	} else if ingressIP == "" {
		s.scope.Info("ingress controller service not found in cluster, not creating ingress DNS record")
		return nil
	}

	input := &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(hostedZoneID),
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{
				s.buildARecordChange("ingress", ingressIP, actionUpsert),
				{
					Action: aws.String(actionUpsert),
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

	baseHostedZone, err := s.findHostedZoneByDNSName(ctx, s.scope.BaseDomain())
	if err != nil {
		return microerror.Mask(err)
	}

	input := &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: baseHostedZone.Id,
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{
				{
					Action: aws.String(action),
					ResourceRecordSet: &route53.ResourceRecordSet{
						Name:            aws.String(s.scope.ClusterDomain()),
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

func (s *Service) upsertClusterRecords(ctx context.Context, hostedZoneID string) error {
	input := &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(hostedZoneID),
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{},
		},
	}

	if s.scope.APIEndpoint() == "" {
		s.scope.Info("API endpoint IP is missing")
		return microerror.Mask(aws.ErrMissingEndpoint)
	}

	s.scope.Info(fmt.Sprintf("found API endpoint IP: %s", s.scope.APIEndpoint()))
	input.ChangeBatch.Changes = append(input.ChangeBatch.Changes,
		s.buildARecordChange("api", s.scope.APIEndpoint(), actionUpsert),
	)

	if s.scope.BastionIP() != "" {
		s.scope.Info(fmt.Sprintf("found bastion IP: %s", s.scope.BastionIP()))
		input.ChangeBatch.Changes = append(input.ChangeBatch.Changes,
			s.buildARecordChange("bastion1", s.scope.BastionIP(), actionUpsert),
		)
	}

	if _, err := s.Route53Client.ChangeResourceRecordSetsWithContext(ctx, input); err != nil {
		return wrapRoute53Error(err)
	}
	return nil
}

func (s *Service) createClusterHostedZone(ctx context.Context) (*route53.HostedZone, error) {
	now := time.Now()
	input := &route53.CreateHostedZoneInput{
		CallerReference: aws.String(now.UTC().String()),
		Name:            aws.String(fmt.Sprintf("%s.", s.scope.ClusterDomain())),
		HostedZoneConfig: &route53.HostedZoneConfig{
			Comment: aws.String(fmt.Sprintf("management_cluster: %s", s.scope.ManagementCluster())),
		},
	}

	output, err := s.Route53Client.CreateHostedZoneWithContext(ctx, input)
	if err != nil {
		return nil, wrapRoute53Error(err)
	}

	return output.HostedZone, nil
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
		if *recordSet.Type == route53.RRTypeSoa || *recordSet.Type == route53.RRTypeNs {
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

func (s *Service) findHostedZoneByDNSName(ctx context.Context, dnsName string) (*route53.HostedZone, error) {
	input := &route53.ListHostedZonesByNameInput{
		DNSName: aws.String(dnsName),
	}
	out, err := s.Route53Client.ListHostedZonesByNameWithContext(ctx, input)
	if err != nil {
		return nil, wrapRoute53Error(err)
	}

	if len(out.HostedZones) == 0 {
		return nil, microerror.Mask(hostedZoneNotFoundError)
	} else if *out.HostedZones[0].Name != fmt.Sprintf("%s.", s.scope.ClusterDomain()) {
		return nil, microerror.Mask(hostedZoneNotFoundError)
	}

	return out.HostedZones[0], nil
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

	ingresses := icService.Status.LoadBalancer.Ingress
	if len(ingresses) < 1 || ingresses[0].IP == "" {
		return "", microerror.Mask(ingressNotReadyError)
	}

	return ingresses[0].IP, nil
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
