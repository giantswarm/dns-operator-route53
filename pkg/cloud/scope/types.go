package scope

import (
	awsclient "github.com/aws/aws-sdk-go/aws/client"
	"github.com/go-logr/logr"
	capo "sigs.k8s.io/cluster-api-provider-openstack/api/v1alpha4"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ClusterScopeParams defines the input parameters used to create a new ClusterScope.
type ClusterScopeParams struct {
	AWSSession       awsclient.ConfigProvider
	ManagementClient client.Client
	Logger           logr.Logger

	BaseDomain            string
	InfrastructureCluster *capo.OpenStackCluster
	ManagementCluster     string
}

// ClusterScope defines the basic context for an actuator to operate upon.
type ClusterScope struct {
	awsSession       awsclient.ConfigProvider
	managementClient client.Client
	logr.Logger

	baseDomain        string
	infraCluster      *capo.OpenStackCluster
	managementCluster string

	workloadClient client.Client
}
