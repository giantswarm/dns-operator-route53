package scope

import (
	awsclient "github.com/aws/aws-sdk-go/aws/client"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/klog/klogr"
	infrav1 "sigs.k8s.io/cluster-api-provider-openstack/api/v1alpha4"
)

// ClusterScopeParams defines the input parameters used to create a new Scope.
type ClusterScopeParams struct {
	OpenstackCluster            *infrav1.OpenStackCluster
	BaseDomain                  string
	ManagementClusterBaseDomain string
	Logger                      logr.Logger
	Region                      string
	Session                     awsclient.ConfigProvider
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
	if params.Logger == (logr.Logger{}) {
		params.Logger = klogr.New()
	}

	if params.ManagementClusterBaseDomain == "" {
		return nil, errors.New("failed to generate new scope from emtpy string ManagementClusterBaseDomain")
	}

	session, err := sessionForRegion(params.Region)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create aws session")
	}

	return &ClusterScope{
		openstackCluster:            params.OpenstackCluster,
		baseDomain:                  params.BaseDomain,
		Logger:                      params.Logger,
		managementClusterBaseDomain: params.ManagementClusterBaseDomain,
		region:                      "readenv",
		session:                     session,
	}, nil
}

// ClusterScope defines the basic context for an actuator to operate upon.
type ClusterScope struct {
	assumeRole       string
	openstackCluster *infrav1.OpenStackCluster
	baseDomain       string
	logr.Logger
	managementClusterBaseDomain string
	region                      string
	session                     awsclient.ConfigProvider
}

// APIEndpoint returns the AWS infrastructure Kubernetes API endpoint.
func (s *ClusterScope) APIEndpoint() string {
	return s.openstackCluster.Spec.ControlPlaneEndpoint.Host
}

// BaseDomain returns the workload cluster basedomain.
func (s *ClusterScope) BaseDomain() string {
	return s.baseDomain
}

// Cluster returns the OpenStack infrastructure cluster.
func (s *ClusterScope) Cluster() *infrav1.OpenStackCluster {
	return s.openstackCluster
}

// ManagementClusterBaseDomain returns the workload cluster basedomain.
func (s *ClusterScope) ManagementClusterBaseDomain() string {
	return s.managementClusterBaseDomain
}

// Name returns the AWS infrastructure cluster name.
func (s *ClusterScope) Name() string {
	return s.openstackCluster.Name
}

// Region returns the cluster region.
func (s *ClusterScope) Region() string {
	return s.region
}

// Session returns the AWS SDK session. Used for creating workload cluster client.
func (s *ClusterScope) Session() awsclient.ConfigProvider {
	return s.session
}
