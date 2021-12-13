package route53

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/giantswarm/microerror"
	"github.com/pkg/errors"

	"github.com/giantswarm/dns-operator-openstack/pkg/cloud/awserrors"
)

var notFoundError = &microerror.Error{
	Kind: "notFoundError",
}

// IsNotFound asserts notFoundError.
func IsNotFound(err error) bool {
	return microerror.Cause(err) == notFoundError || microerror.Cause(err) == hostedZoneNotFoundError
}

var hostedZoneNotFoundError = &microerror.Error{
	Kind: "hostedZoneNotFoundError",
}

// IsHostedZoneNotFound asserts hostedZoneNotFoundError.
func IsHostedZoneNotFound(err error) bool {
	return microerror.Cause(err) == hostedZoneNotFoundError
}

var serviceNotReadyError = &microerror.Error{
	Kind: "serviceNotReadyError",
}

// IsServiceNotReady asserts serviceNotReadyError.
func IsServiceNotReady(err error) bool {
	return microerror.Cause(err) == serviceNotReadyError
}

func wrapRoute53Error(err error) error {
	if err == aws.ErrMissingEndpoint {
		return microerror.Mask(notFoundError)
	}

	if code, ok := awserrors.Code(errors.Cause(err)); ok {
		if code == route53.ErrCodeHostedZoneNotFound || code == route53.ErrCodeInvalidChangeBatch {
			return microerror.Mask(notFoundError)
		}
	}

	return microerror.Mask(err)
}
