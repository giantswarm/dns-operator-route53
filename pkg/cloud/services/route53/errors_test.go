package route53

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/pkg/errors"
)

func TestWrapRoute53Error(t *testing.T) {
	for _, tc := range []struct {
		name  string
		err   error
		check func(error) bool
	}{
		{
			name:  "a hosted zone which is not found",
			err:   awserr.New(route53.ErrCodeHostedZoneNotFound, "hosted zone not found", nil),
			check: IsHostedZoneNotFound,
		},
		{
			name:  "an invalid change batch for a record which is not found",
			err:   awserr.New(route53.ErrCodeInvalidChangeBatch, "the record was not found", nil),
			check: IsNotFound,
		},
		{
			name:  "a throttled request",
			err:   awserr.New(route53.ErrCodeThrottlingException, "slow down", nil),
			check: IsThrottlingRateExceededError,
		},
		{
			name: "an invalid change batch for another reason",
			err:  awserr.New(route53.ErrCodeInvalidChangeBatch, "the batch is too large", nil),
			check: func(err error) bool {
				return !IsNotFound(err) && !IsHostedZoneNotFound(err)
			},
		},
		{
			name: "an error which does not come from AWS",
			err:  errors.New("connection refused"),
			check: func(err error) bool {
				return !IsNotFound(err) && !IsHostedZoneNotFound(err) && !IsThrottlingRateExceededError(err)
			},
		},
		{
			// The Route53 API answers with this code when a zone ID is unknown,
			// and the operator does not translate it.
			name: "a hosted zone which does not exist",
			err:  awserr.New(route53.ErrCodeNoSuchHostedZone, "no such hosted zone", nil),
			check: func(err error) bool {
				return !IsHostedZoneNotFound(err)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := wrapRoute53Error(tc.err); !tc.check(got) {
				t.Errorf("wrapRoute53Error(%v) = %v, which is the wrong kind", tc.err, got)
			}
		})
	}
}

func TestWrapRoute53ErrorKeepsTheMessage(t *testing.T) {
	err := wrapRoute53Error(errors.New("connection refused"))

	if err == nil || err.Error() == "" {
		t.Fatalf("wrapRoute53Error lost the message")
	}
}
