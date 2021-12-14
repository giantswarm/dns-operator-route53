package route53

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/giantswarm/microerror"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	IngressAppPrefix    = "nginx-ingress-controller-app-"
	IngressAppNamespace = "kube-system"
	TTL                 = 300
)

func (s *Service) DeleteRoute53() error {
	s.scope.V(2).Info("Deleting hosted DNS zone")
	hostedZoneID, err := s.describeClusterHostedZone()
	if IsHostedZoneNotFound(err) {
		return nil
	} else if err != nil {
		return microerror.Mask(err)
	}

	// We need to delete all records first before we can delete the hosted zone
	if err := s.changeClusterIngressRecords("DELETE"); err != nil {
		return microerror.Mask(err)
	}

	if err := s.changeClusterAPIRecords("DELETE"); err != nil {
		return microerror.Mask(err)
	}

	// Then delete delegation record in base hosted zone
	if err := s.changeClusterNSDelegation("DELETE"); err != nil {
		return microerror.Mask(err)
	}

	// Finally delete DNS zone for cluster
	err = s.deleteClusterHostedZone(hostedZoneID)
	if err != nil {
		return microerror.Mask(err)
	}
	s.scope.V(2).Info(fmt.Sprintf("Deleting hosted zone completed successfully for cluster %s", s.scope.Name()))
	return nil
}

func (s *Service) ReconcileRoute53() error {
	s.scope.V(2).Info("Reconciling hosted DNS zone")

	// Describe or create.
	_, err := s.describeClusterHostedZone()
	if IsHostedZoneNotFound(err) {
		err = s.createClusterHostedZone()
		if err != nil {
			return microerror.Mask(err)
		}
		s.scope.Info(fmt.Sprintf("Created new hosted zone for cluster %s", s.scope.Name()))
	} else if err != nil {
		return microerror.Mask(err)
	}

	err = s.changeClusterNSDelegation("CREATE")
	if IsAlreadyExists(err) {
		// Fall through
	} else if err != nil {
		return microerror.Mask(err)
	}

	err = s.changeClusterAPIRecords("CREATE")
	if IsAlreadyExists(err) {
		// Fall through
	} else if err != nil {
		return microerror.Mask(err)
	}

	err = s.changeClusterIngressRecords("CREATE")
	if IsAlreadyExists(err) {
		// Fall through
	} else if err != nil {
		return microerror.Mask(err)
	}

	return nil
}

func (s *Service) describeClusterHostedZone() (string, error) {
	// Search host zone by DNSName
	input := &route53.ListHostedZonesByNameInput{
		DNSName: aws.String(fmt.Sprintf("%s.%s", s.scope.Name(), s.scope.BaseDomain())),
	}
	out, err := s.Route53Client.ListHostedZonesByName(input)
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

func (s *Service) listClusterNSRecords() ([]*route53.ResourceRecord, error) {
	hostZoneID, err := s.describeClusterHostedZone()
	if err != nil {
		return nil, microerror.Mask(err)
	}

	// First entry is always NS record
	input := &route53.ListResourceRecordSetsInput{
		HostedZoneId: aws.String(hostZoneID),
		MaxItems:     aws.String("1"),
	}

	output, err := s.Route53Client.ListResourceRecordSets(input)
	if err != nil {
		return nil, wrapRoute53Error(err)
	}
	return output.ResourceRecordSets[0].ResourceRecords, nil
}

func (s *Service) changeClusterAPIRecords(action string) error {
	s.scope.Info(s.scope.APIEndpoint())
	if s.scope.APIEndpoint() == "" {
		s.scope.Info("API endpoint is not ready yet.")
		return aws.ErrMissingEndpoint
	}

	hostZoneID, err := s.describeClusterHostedZone()
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

	_, err = s.Route53Client.ChangeResourceRecordSets(input)
	if err != nil {
		return wrapRoute53Error(err)
	}
	return nil
}

func (s *Service) changeClusterIngressRecords(action string) error {
	hostZoneID, err := s.describeClusterHostedZone()
	if err != nil {
		return microerror.Mask(err)
	}

	ip, err := s.getIngressIP()
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
						Name: aws.String(fmt.Sprintf("ingress.%s.%s", s.scope.Name(), s.scope.BaseDomain())),
						Type: aws.String("A"),
						TTL:  aws.Int64(TTL),
						ResourceRecords: []*route53.ResourceRecord{
							{
								Value: aws.String(ip),
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

	_, err = s.Route53Client.ChangeResourceRecordSets(input)
	if err != nil {
		return wrapRoute53Error(err)
	}
	return nil
}

func (s *Service) describeBaseHostedZone() (string, error) {
	input := &route53.ListHostedZonesByNameInput{
		DNSName: aws.String(s.scope.BaseDomain()),
	}
	out, err := s.Route53Client.ListHostedZonesByName(input)
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

func (s *Service) changeClusterNSDelegation(action string) error {
	hostZoneID, err := s.describeBaseHostedZone()
	if err != nil {
		return microerror.Mask(err)
	}

	records, err := s.listClusterNSRecords()
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

	_, err = s.Route53Client.ChangeResourceRecordSets(input)
	if err != nil {
		return wrapRoute53Error(err)
	}
	return nil
}

func (s *Service) createClusterHostedZone() error {
	now := time.Now()
	input := &route53.CreateHostedZoneInput{
		CallerReference: aws.String(now.UTC().String()),
		Name:            aws.String(fmt.Sprintf("%s.%s.", s.scope.Name(), s.scope.BaseDomain())),
	}
	_, err := s.Route53Client.CreateHostedZone(input)
	if err != nil {
		return wrapRoute53Error(err)
	}
	return nil
}

func (s *Service) deleteClusterHostedZone(hostedZoneID string) error {
	input := &route53.DeleteHostedZoneInput{
		Id: aws.String(hostedZoneID),
	}
	_, err := s.Route53Client.DeleteHostedZone(input)
	if err != nil {
		return wrapRoute53Error(err)
	}
	return nil
}

func (s *Service) getIngressIP() (string, error) {
	serviceName := fmt.Sprintf("%s%s", IngressAppPrefix, s.scope.Name())
	icService, err := s.scope.ClusterK8sClient().CoreV1().Services(IngressAppNamespace).Get(context.Background(), serviceName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return "", microerror.Mask(ingressNotReadyError)
	} else if err != nil {
		return "", microerror.Mask(err)
	}

	if len(icService.Status.LoadBalancer.Ingress) < 1 || icService.Status.LoadBalancer.Ingress[0].IP == "" {
		return "", microerror.Mask(ingressNotReadyError)
	}

	return icService.Status.LoadBalancer.Ingress[0].IP, nil

}
