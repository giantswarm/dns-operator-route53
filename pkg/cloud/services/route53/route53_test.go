package route53

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/route53"
)

func TestRequiresUpdate(t *testing.T) {
	const endpoint = "10.0.0.1"

	for _, tc := range []struct {
		name     string
		set      *route53.ResourceRecordSet
		endpoint string
		want     bool
	}{
		{
			name:     "no resource records",
			set:      &route53.ResourceRecordSet{},
			endpoint: endpoint,
			want:     true,
		},
		{
			name:     "an empty list of resource records",
			set:      &route53.ResourceRecordSet{ResourceRecords: []*route53.ResourceRecord{}},
			endpoint: endpoint,
			want:     true,
		},
		{
			name: "a resource record without a value",
			set: &route53.ResourceRecordSet{
				ResourceRecords: []*route53.ResourceRecord{{}},
			},
			endpoint: endpoint,
			want:     true,
		},
		{
			name: "a different value",
			set: &route53.ResourceRecordSet{
				ResourceRecords: []*route53.ResourceRecord{{Value: aws.String("10.0.0.2")}},
			},
			endpoint: endpoint,
			want:     true,
		},
		{
			name: "the same value",
			set: &route53.ResourceRecordSet{
				ResourceRecords: []*route53.ResourceRecord{{Value: aws.String(endpoint)}},
			},
			endpoint: endpoint,
			want:     false,
		},
		{
			name: "an empty endpoint and an empty value",
			set: &route53.ResourceRecordSet{
				ResourceRecords: []*route53.ResourceRecord{{Value: aws.String("")}},
			},
			endpoint: "",
			want:     false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := requiresUpdate(tc.set, tc.endpoint); got != tc.want {
				t.Errorf("requiresUpdate = %v, want %v", got, tc.want)
			}
		})
	}
}
