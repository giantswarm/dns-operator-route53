package scope

import (
	awsclient "github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/klog/klogr"
	capo "sigs.k8s.io/cluster-api-provider-openstack/api/v1alpha4"
)

// ClusterScopeParams defines the input parameters used to create a new Scope.
type ClusterScopeParams struct {
	OpenstackCluster *capo.OpenStackCluster
	BaseDomain       string
	Logger           logr.Logger
	Session          awsclient.ConfigProvider
}

// NewClusterScope creates a new Scope from the supplied parameters.
// This is meant to be called for each reconcile iteration.
func NewClusterScope(params ClusterScopeParams) (*ClusterScope, error) {
	if params.OpenstackCluster == nil {
		return nil, errors.New("failed to generate new scope from nil OpenstackCluster")
	}
	if params.BaseDomain == "" {
		return nil, errors.New("failed to generate new scope from emtpy string BaseDomain")
	}
	if params.Logger == nil {
		params.Logger = klogr.New()
	}

	session, err := session.NewSession()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create aws session")
	}

	return &ClusterScope{
		openstackCluster: params.OpenstackCluster,
		baseDomain:       params.BaseDomain,
		Logger:           params.Logger,
		session:          session,
	}, nil
}

// ClusterScope defines the basic context for an actuator to operate upon.
type ClusterScope struct {
	openstackCluster *capo.OpenStackCluster
	baseDomain       string
	logr.Logger
	session awsclient.ConfigProvider
}

// APIEndpoint returns the AWS infrastructure Kubernetes API endpoint.
func (s *ClusterScope) APIEndpoint() string {
	return s.openstackCluster.Spec.ControlPlaneEndpoint.Host
}

// BaseDomain returns the cluster basedomain.
func (s *ClusterScope) BaseDomain() string {
	return s.baseDomain
}

// Cluster returns the OpenStack infrastructure cluster.
func (s *ClusterScope) Cluster() *capo.OpenStackCluster {
	return s.openstackCluster
}

// Name returns the AWS infrastructure cluster name.
func (s *ClusterScope) Name() string {
	return s.openstackCluster.Name
}

// Session returns the AWS SDK session. Used for creating cluster client.
func (s *ClusterScope) Session() awsclient.ConfigProvider {
	return s.session
}
