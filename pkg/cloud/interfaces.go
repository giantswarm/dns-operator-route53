package cloud

import (
	"context"

	awsclient "github.com/aws/aws-sdk-go/aws/client"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
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
	Session

	// APIEndpoint returns the LoadBalancer API endpoint for the cluster.
	// e.g. apiserver-x.eu-central-1.elb.amazonaws.com
	APIEndpoint() string
	// BaseDomain returns the base domain.
	BaseDomain() string
	// BastionIP returns the bastion IP.
	BastionIP() string
	// Cluster returns the CAPI cluster.
	Cluster() *capi.Cluster
	// ClusterK8sClient returns a client to interact with the cluster.
	ClusterK8sClient(ctx context.Context) (client.Client, error)
	// ClusterDomain returns the cluster domain.
	ClusterDomain() string
	// InfrastructureCluster returns the unstructured InfrastructureCluster
	InfrastructureCluster() *unstructured.Unstructured
	// ManagementCluster returns the name of the management cluster.
	ManagementCluster() string
	// Name returns the CAPI cluster name.
	Name() string
	// WildcardCNAMETarget returns the override value for the wildcard CNAME record,
	// or empty string to use the default ingress.<clusterdomain> value.
	WildcardCNAMETarget() string
}
