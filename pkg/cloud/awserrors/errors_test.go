package awserrors

import (
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go/aws/awserr"
)

func TestCode(t *testing.T) {
	for _, tc := range []struct {
		name     string
		err      error
		wantCode string
		wantOK   bool
	}{
		{
			name:     "an AWS error",
			err:      awserr.New("Throttling", "slow down", nil),
			wantCode: "Throttling",
			wantOK:   true,
		},
		{
			name:   "an error which does not come from AWS",
			err:    errors.New("connection refused"),
			wantOK: false,
		},
		{
			name:   "no error",
			err:    nil,
			wantOK: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, ok := Code(tc.err)

			if ok != tc.wantOK {
				t.Errorf("ok = %v, want %v", ok, tc.wantOK)
			}
			if code != tc.wantCode {
				t.Errorf("code = %q, want %q", code, tc.wantCode)
			}
		})
	}
}
