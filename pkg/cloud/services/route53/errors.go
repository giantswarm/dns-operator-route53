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

// IsHostedZoneNotFound asserts hostedZoneNotFoundError.
func IsHostedZoneNotFound(err error) bool {
	return microerror.Cause(err) == hostedZoneNotFoundError
}

var hostedZoneNotFoundError = &microerror.Error{
	Kind: "hostedZoneNotFoundError",
}

func IsThrottlingRateExceededError(err error) bool {
	return microerror.Cause(err) == rateLimitHitError
}

var rateLimitHitError = &microerror.Error{
	Kind: "throttlingRateExceededError",
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
		case route53.ErrCodeThrottlingException:
			return microerror.Mask(rateLimitHitError)
		}
	}

	return microerror.Mask(err)
}

// check later - imo no need to wrap the errors
