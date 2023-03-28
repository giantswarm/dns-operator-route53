package route53

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/allegro/bigcache/v3"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/giantswarm/microerror"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	dnscache "github.com/giantswarm/dns-operator-route53/pkg/cloud/cache"
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

	log := log.FromContext(ctx)
	log.Info("Deleting hosted DNS zone")

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
	log.Info(fmt.Sprintf("Deleting hosted zone completed successfully for cluster %s", s.scope.Name()))
	return nil
}

func (s *Service) ReconcileRoute53(ctx context.Context) error {
	log := log.FromContext(ctx)
	log.Info("Reconciling hosted DNS zone")

	cachedHostedZoneID, err := dnscache.GetDNSCacheRecord(dnscache.ZoneID, s.scope.Name())
	if errors.Is(err, bigcache.ErrEntryNotFound) {
		log.Info(fmt.Sprintf("no hostedZoneID found in local cache for cluster %s", s.scope.Name()))
		// Describe or create.
		hostedZoneID, err := s.describeClusterHostedZone(ctx)
		if IsHostedZoneNotFound(err) {
			hostedZoneID, err = s.createClusterHostedZone(ctx)
			if err != nil {
				return microerror.Mask(err)
			}
			log.Info(fmt.Sprintf("Created new hosted zone for cluster %s", s.scope.Name()))
		} else if err != nil {
			return microerror.Mask(err)
		}

		if err := dnscache.SetDNSCacheRecord(dnscache.ZoneID, s.scope.Name(), []byte(hostedZoneID)); err != nil {
			return err
		}
		cachedHostedZoneID, err = dnscache.GetDNSCacheRecord(dnscache.ZoneID, s.scope.Name())
		if err != nil {
			return err
		}
	}

	if err := s.changeClusterNSDelegation(ctx, string(cachedHostedZoneID), actionUpsert); err != nil {
		return microerror.Mask(err)
	}

	if err := s.changeClusterRecords(ctx, string(cachedHostedZoneID), actionUpsert); err != nil {
		return microerror.Mask(err)
	}

	if err := s.changeClusterIngressRecords(ctx, string(cachedHostedZoneID), actionUpsert); err != nil {
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
		// Ingress service is not installed in this cluster.
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

	cachedClusterIngressRecords, _ := dnscache.GetDNSCacheRecord(dnscache.ClusterIngressRecords, hostedZoneID)
	if input.String() != string(cachedClusterIngressRecords) {
		if err = dnscache.SetDNSCacheRecord(dnscache.ClusterIngressRecords, hostedZoneID, []byte(input.String())); err != nil {
			return err
		}

		if _, err := s.Route53Client.ChangeResourceRecordSetsWithContext(ctx, input); err != nil {
			return wrapRoute53Error(err)
		}
	}

	return nil
}

func (s *Service) changeClusterNSDelegation(ctx context.Context, hostedZoneID, action string) error {
	log := log.FromContext(ctx)

	var resourceRecords []*route53.ResourceRecord
	cachedRecords, err := dnscache.GetDNSCacheRecord(dnscache.NameserverRecords, hostedZoneID)
	if errors.Is(err, bigcache.ErrEntryNotFound) {
		log.V(4).Info(fmt.Sprintf("no cached name server records found for zone %s", hostedZoneID))

		resourceRecords, err = s.listClusterNSRecords(ctx, hostedZoneID)
		if err != nil {
			return microerror.Mask(err)
		}

		jsonRecords, _ := json.Marshal(resourceRecords)
		if err = dnscache.SetDNSCacheRecord(dnscache.NameserverRecords, hostedZoneID, []byte(string(jsonRecords))); err != nil {
			return err
		}
		cachedRecords, err = dnscache.GetDNSCacheRecord(dnscache.NameserverRecords, hostedZoneID)
		if err != nil {
			return err
		}
	}

	if err := json.Unmarshal(cachedRecords, &resourceRecords); err != nil {
		return err
	}

	cachedBaseHostedZoneID, err := dnscache.GetDNSCacheRecord(dnscache.ZoneID, s.scope.ClusterDomain())
	if errors.Is(err, bigcache.ErrEntryNotFound) {
		log.Info(fmt.Sprintf("no cached zone id found for domain %s", s.scope.ClusterDomain()))

		baseHostedZoneID, err := s.describeBaseHostedZone(ctx)
		if err != nil {
			return microerror.Mask(err)
		}

		if err = dnscache.SetDNSCacheRecord(dnscache.ZoneID, s.scope.ClusterDomain(), []byte(baseHostedZoneID)); err != nil {
			return err
		}
		cachedBaseHostedZoneID, err = dnscache.GetDNSCacheRecord(dnscache.ZoneID, s.scope.ClusterDomain())
		if err != nil {
			return err
		}
	}

	input := &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(string(cachedBaseHostedZoneID)),
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{
				{
					Action: aws.String(action),
					ResourceRecordSet: &route53.ResourceRecordSet{
						Name:            aws.String(s.scope.ClusterDomain()),
						Type:            aws.String("NS"),
						TTL:             aws.Int64(ttl),
						ResourceRecords: resourceRecords,
					},
				},
			},
		},
	}

	cachedBaseHostedZoneIDRecords, _ := dnscache.GetDNSCacheRecord(dnscache.ZoneRecords, s.scope.ClusterDomain())

	// if cached input differ from computed input
	if input.String() != string(cachedBaseHostedZoneIDRecords) {
		log.Info(fmt.Sprintf("cached records for zone ID %s differs from computed records. Updating ResourceRecordSet", string(cachedBaseHostedZoneID)))

		if err := dnscache.SetDNSCacheRecord(dnscache.ZoneRecords, s.scope.ClusterDomain(), []byte(input.String())); err != nil {
			return err
		}

		_, err := s.Route53Client.ChangeResourceRecordSetsWithContext(ctx, input)
		if err != nil {
			return wrapRoute53Error(err)
		}
	}

	return nil
}

func (s *Service) changeClusterRecords(ctx context.Context, hostedZoneID string, action string) error {
	log := log.FromContext(ctx)
	input := &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(hostedZoneID),
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{},
		},
	}

	cachedHostedZoneIDRecordSets, err := dnscache.GetDNSCacheRecord(dnscache.ZoneRecords, hostedZoneID)

	if errors.Is(err, bigcache.ErrEntryNotFound) {

		log.Info(fmt.Sprintf("no cached resource record set found for zone id %s", hostedZoneID))

		recordSets, err := s.listResourceRecordSets(ctx, hostedZoneID)
		if err != nil {
			return wrapRoute53Error(err)
		}

		jsonRecords, _ := json.Marshal(recordSets.ResourceRecordSets)
		if err = dnscache.SetDNSCacheRecord(dnscache.ZoneRecords, hostedZoneID, jsonRecords); err != nil {
			return err
		}
		cachedHostedZoneIDRecordSets, err = dnscache.GetDNSCacheRecord(dnscache.ZoneRecords, hostedZoneID)
		if err != nil {
			return err
		}
	}

	var recordSets []*route53.ResourceRecordSet
	if err := json.Unmarshal(cachedHostedZoneIDRecordSets, &recordSets); err != nil {
		return err
	}

	// check if we already have entries for the supported "core" endpoints
	// kubernetes API IP & bastion host IP
	var endpointIPs struct {
		kubernetesAPIRequiresUpdate bool
		bastionIPRequiresUpdate     bool
	}
	for _, recordSet := range recordSets {
		if *recordSet.Name == "api"+"."+s.scope.Name()+"."+s.scope.BaseDomain()+"." {
			endpointIPs.kubernetesAPIRequiresUpdate = requiresUpdate(recordSet, s.scope.APIEndpoint())
		}
		if *recordSet.Name == "bastion1"+"."+s.scope.Name()+"."+s.scope.BaseDomain()+"." {
			endpointIPs.bastionIPRequiresUpdate = requiresUpdate(recordSet, s.scope.BastionIP())
		}
	}

	// decide if we need changes
	if s.scope.APIEndpoint() == "" {
		log.Info("API endpoint is not ready yet.")
		return aws.ErrMissingEndpoint
	} else if s.scope.APIEndpoint() != "" && endpointIPs.kubernetesAPIRequiresUpdate {
		input.ChangeBatch.Changes = append(input.ChangeBatch.Changes,
			s.buildARecordChange(hostedZoneID, "api", s.scope.APIEndpoint(), actionUpsert),
		)
	} else if s.scope.BastionIP() != "" && endpointIPs.bastionIPRequiresUpdate {
		input.ChangeBatch.Changes = append(input.ChangeBatch.Changes,
			s.buildARecordChange(hostedZoneID, "bastion1", s.scope.BastionIP(), actionUpsert),
		)
	} else if s.scope.BastionIP() == "" {
		for _, recordSet := range recordSets {
			if *recordSet.Name == "bastion1"+"."+s.scope.Name()+"."+s.scope.BaseDomain()+"." {
				log.Info("orphaned bastion record found", "name", *recordSet.Name, "record", *recordSet.ResourceRecords[0].Value)
				input.ChangeBatch.Changes = append(input.ChangeBatch.Changes,
					s.buildARecordChange(hostedZoneID, "bastion1", *recordSet.ResourceRecords[0].Value, actionDelete),
				)
			}
		}
	}

	if len(input.ChangeBatch.Changes) > 0 {
		// invalidate the cache
		if err = dnscache.DeleteDNSCacheRecord(dnscache.ZoneRecords, hostedZoneID); err != nil {
			return err
		}

		if _, err := s.Route53Client.ChangeResourceRecordSetsWithContext(ctx, input); err != nil {
			return wrapRoute53Error(err)
		}
	}

	return nil
}

func requiresUpdate(set *route53.ResourceRecordSet, endpoint string) bool {
	if set.ResourceRecords == nil {
		return true
	}

	if len(set.ResourceRecords) == 0 {
		return true
	}

	if set.ResourceRecords[0].Value == nil {
		return true
	}

	return *set.ResourceRecords[0].Value != endpoint
}

func (s *Service) listResourceRecordSets(ctx context.Context, hostedZoneID string) (*route53.ListResourceRecordSetsOutput, error) {
	listParams := &route53.ListResourceRecordSetsInput{
		HostedZoneId: aws.String(hostedZoneID),
	}
	respList, err := s.Route53Client.ListResourceRecordSets(listParams)
	if err != nil {
		return &route53.ListResourceRecordSetsOutput{}, wrapRoute53Error(err)
	}
	return respList, nil
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

	// delete cached records for given Zone
	if err = dnscache.DeleteDNSCacheRecord(dnscache.ZoneRecords, hostedZoneID); err != nil {
		return err
	}

	// delete cached zoneID for cluster
	if err = dnscache.DeleteDNSCacheRecord(dnscache.ZoneID, s.scope.Name()); err != nil {
		return err
	}

	return nil
}

func (s *Service) describeBaseHostedZone(ctx context.Context) (string, error) {
	input := &route53.ListHostedZonesByNameInput{
		DNSName: aws.String(s.scope.BaseDomain()),
	}
	out, err := s.Route53Client.ListHostedZonesByNameWithContext(ctx, input)
	if err != nil {
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

	var icServiceIP string

	for _, icService := range icServices.Items {
		if icService.Spec.Type == corev1.ServiceTypeLoadBalancer {
			if icServiceIP != "" {
				return "", microerror.Mask(tooManyICServicesError)
			}

			if len(icService.Status.LoadBalancer.Ingress) < 1 || icService.Status.LoadBalancer.Ingress[0].IP == "" {
				return "", microerror.Mask(ingressNotReadyError)
			}

			icServiceIP = icService.Status.LoadBalancer.Ingress[0].IP
		}
	}

	return icServiceIP, nil
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
