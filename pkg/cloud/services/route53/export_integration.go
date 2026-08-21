//go:build integration

package route53

import (
	"github.com/aws/aws-sdk-go/service/route53/route53iface"

	"github.com/giantswarm/dns-operator-route53/pkg/cloud/scope"
)

// NewServiceWithClient returns a new service which uses the given route53 api
// client. The scope field is unexported, so the controller tests need this
// constructor. It only exists under the integration build tag, which keeps it
// out of the production binary and out of the public API of this package.
func NewServiceWithClient(clusterScope scope.Route53Scope, client route53iface.Route53API) *Service {
	return &Service{
		scope:         clusterScope,
		Route53Client: client,
	}
}
