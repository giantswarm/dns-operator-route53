package route53

import (
	"strings"

	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/pkg/errors"

	"github.com/giantswarm/microerror"

	"github.com/giantswarm/dns-operator-route53/pkg/cloud/awserrors"
)

// IsNotFound asserts notFoundError.
func IsNotFound(err error) bool {
	return microerror.Cause(err) == notFoundError
}

var notFoundError = &microerror.Error{
	Kind: "notFoundError",
}

var hostedZoneNotFoundError = &microerror.Error{
	Kind: "hostedZoneNotFoundError",
}

// IsHostedZoneNotFound asserts hostedZoneNotFoundError.
func IsHostedZoneNotFound(err error) bool {
	return microerror.Cause(err) == hostedZoneNotFoundError
}

var ingressNotReadyError = &microerror.Error{
	Kind: "ingressNotReadyError",
}

// IsIngressNotRead asserts ingressNotReadyError.
func IsIngressNotReady(err error) bool {
	return microerror.Cause(err) == ingressNotReadyError
}

var tooManyICServicesError = &microerror.Error{
	Kind: "tooManyICServicesError",
}

// IsTooManyICServices asserts tooManyICServicesError.
func IsTooManyICServices(err error) bool {
	return microerror.Cause(err) == tooManyICServicesError
}

func wrapRoute53Error(err error) error {
	if code, ok := awserrors.Code(errors.Cause(err)); ok {
		switch code {
		case route53.ErrCodeHostedZoneNotFound:
			return microerror.Mask(hostedZoneNotFoundError)
		case route53.ErrCodeInvalidChangeBatch:
			if strings.Contains(err.Error(), "not found") {
				return microerror.Mask(notFoundError)
			}
		}
	}

	return microerror.Mask(err)
}
