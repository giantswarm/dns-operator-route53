package metrics

import (
	"testing"
)

func TestEndpointToService(t *testing.T) {
	for _, tc := range []struct {
		name     string
		endpoint string
		want     string
	}{
		{
			name:     "a route53 endpoint",
			endpoint: "https://route53.amazonaws.com",
			want:     "route53",
		},
		{
			name:     "a regional endpoint",
			endpoint: "https://sts.eu-central-1.amazonaws.com",
			want:     "sts",
		},
		{
			name:     "an endpoint without a scheme",
			endpoint: "route53.amazonaws.com",
			want:     "",
		},
		{
			name:     "an empty endpoint",
			endpoint: "",
			want:     "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := endpointToService(tc.endpoint); got != tc.want {
				t.Errorf("endpointToService(%q) = %q, want %q", tc.endpoint, got, tc.want)
			}
		})
	}
}
