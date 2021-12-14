package route53

import (
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/giantswarm/microerror"
	"github.com/pkg/errors"

	"github.com/giantswarm/dns-operator-openstack/pkg/cloud/awserrors"
)

var alreadyExistsError = &microerror.Error{
	Kind: "alreadyExistsError",
}

// IsAlreadyExists asserts alreadyExistsError.
func IsAlreadyExists(err error) bool {
	return microerror.Cause(err) == alreadyExistsError
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
	// if err == aws.ErrMissingEndpoint {
	// 	return microerror.Mask(notFoundError)
	// }

	if code, ok := awserrors.Code(errors.Cause(err)); ok {
		switch code {
		case route53.ErrCodeHostedZoneNotFound:
			return microerror.Mask(hostedZoneNotFoundError)
		case route53.ErrCodeInvalidChangeBatch:
			return microerror.Mask(alreadyExistsError)
		}
	}

	return microerror.Mask(err)
}
