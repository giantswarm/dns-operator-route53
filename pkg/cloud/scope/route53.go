package scope

import (
	"github.com/giantswarm/dns-operator-openstack/pkg/cloud"
)

// Route53Scope is a scope for use with the Route53 reconciling service
type Route53Scope interface {
	cloud.ClusterScoper
}
