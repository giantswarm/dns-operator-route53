package route53

import (
	"fmt"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/giantswarm/microerror"
)

const (
	TTL = 300
)

func (s *Service) DeleteRoute53() error {
	s.scope.V(2).Info("Deleting hosted DNS zone")
	hostedZoneID, err := s.describeClusterHostedZone()
	if IsNotFound(err) {
		return nil
	} else if err != nil {
		return microerror.Mask(err)
	}

	// First delete delegation record in base hosted zone
	if err := s.changeClusterNSDelegation("DELETE"); err != nil {
		return microerror.Mask(err)
	}

	// We need to delete all records first before we can delete the hosted zone
	if err := s.changeClusterRecords("DELETE"); err != nil {
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
	if IsNotFound(err) {
		err = s.createClusterHostedZone()
		if err != nil {
			return microerror.Mask(err)
		}
		s.scope.Info(fmt.Sprintf("Created new hosted zone for cluster %s", s.scope.Name()))
	} else if err != nil {
		return microerror.Mask(err)
	}

	err = s.changeClusterRecords("CREATE")
	if IsNotFound(err) {
		// Fall through
	} else if err != nil {
		return microerror.Mask(err)
	}

	err = s.changeClusterNSDelegation("CREATE")
	if IsNotFound(err) {
		return nil
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
		return "", microerror.Mask(err)
	}
	if len(out.HostedZones) == 0 {
		return "", &Route53Error{Code: http.StatusNotFound, msg: route53.ErrCodeHostedZoneNotFound}
	}

	if *out.HostedZones[0].Name != fmt.Sprintf("%s.%s.", s.scope.Name(), s.scope.BaseDomain()) {
		return "", &Route53Error{Code: http.StatusNotFound, msg: route53.ErrCodeHostedZoneNotFound}
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
		return nil, microerror.Mask(err)
	}
	return output.ResourceRecordSets[0].ResourceRecords, nil
}

func (s *Service) changeClusterRecords(action string) error {
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
						Name: aws.String(fmt.Sprintf("*.%s.%s", s.scope.Name(), s.scope.BaseDomain())),
						Type: aws.String("CNAME"),
						TTL:  aws.Int64(TTL),
						ResourceRecords: []*route53.ResourceRecord{
							{
								Value: aws.String(fmt.Sprintf("ingress.%s.%s", s.scope.Name(), s.scope.BaseDomain())),
							},
						},
					},
				},
				{
					Action: aws.String(action),
					ResourceRecordSet: &route53.ResourceRecordSet{
						Name: aws.String(fmt.Sprintf("api.%s.%s", s.scope.Name(), s.scope.BaseDomain())),
						Type: aws.String("A"),
						TTL:  aws.Int64(TTL),
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
		return microerror.Mask(err)
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
		return "", microerror.Mask(err)
	}
	if len(out.HostedZones) == 0 {
		return "", &Route53Error{Code: http.StatusNotFound, msg: route53.ErrCodeHostedZoneNotFound}
	}

	if *out.HostedZones[0].Name != fmt.Sprintf("%s.", s.scope.BaseDomain()) {
		return "", &Route53Error{Code: http.StatusNotFound, msg: route53.ErrCodeHostedZoneNotFound}
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
		return microerror.Mask(err)
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
		return microerror.Mask(err)
	}
	return nil
}

func (s *Service) deleteClusterHostedZone(hostedZoneID string) error {
	input := &route53.DeleteHostedZoneInput{
		Id: aws.String(hostedZoneID),
	}
	_, err := s.Route53Client.DeleteHostedZone(input)
	if err != nil {
		return microerror.Mask(err)
	}
	return nil
}
