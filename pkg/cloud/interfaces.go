package cloud

import (
	"context"

	awsclient "github.com/aws/aws-sdk-go/aws/client"
	"github.com/go-logr/logr"
	capo "sigs.k8s.io/cluster-api-provider-openstack/api/v1alpha4"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Session represents an AWS session
type Session interface {
	Session() awsclient.ConfigProvider
}

// ClusterObject represents a AWS cluster object
type ClusterObject interface {
	conditions.Setter
}

// ClusterScoper is the interface for a cluster scope
type ClusterScoper interface {
	logr.Logger
	Session

	// APIEndpoint returns the OpenStack LoadBalancer API endpoint for the cluster.
	// e.g. apiserver-x.eu-central-1.elb.amazonaws.com
	APIEndpoint() string
	// BaseDomain returns the base domain.
	BaseDomain() string
	// ClusterK8sClient returns a client to interact with the cluster.
	ClusterK8sClient(ctx context.Context) (client.Client, error)
	// InfrastructureCluster returns the infrastructure cluster object.
	InfrastructureCluster() *capo.OpenStackCluster
	// ManagementCluster returns the name of the management cluster.
	ManagementCluster() string
	// Name returns the CAPI cluster name.
	Name() string
}
