package scope

import (
	"context"
	"encoding/base64"
	"fmt"

	awsclient "github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/giantswarm/k8sclient/v6/pkg/k8sclient"
	"github.com/giantswarm/k8sclient/v6/pkg/k8srestconfig"
	"github.com/giantswarm/microerror"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
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

	clusterK8sClient, err := getClusterK8sClient(params.OpenstackCluster)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get kubernetes client for cluster")
	}

	return &ClusterScope{
		openstackCluster: params.OpenstackCluster,
		baseDomain:       params.BaseDomain,
		k8sClient:        clusterK8sClient,
		Logger:           params.Logger,
		session:          session,
	}, nil
}

// ClusterScope defines the basic context for an actuator to operate upon.
type ClusterScope struct {
	openstackCluster *capo.OpenStackCluster
	baseDomain       string
	k8sClient        kubernetes.Interface
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

// ClusterK8sClient returns a client to interact with the cluster.
func (s *ClusterScope) ClusterK8sClient() kubernetes.Interface {
	return s.k8sClient
}

// Name returns the AWS infrastructure cluster name.
func (s *ClusterScope) Name() string {
	return s.openstackCluster.Name
}

// Session returns the AWS SDK session. Used for creating cluster client.
func (s *ClusterScope) Session() awsclient.ConfigProvider {
	return s.session
}

func getClusterK8sClient(cluster *capo.OpenStackCluster) (kubernetes.Interface, error) {
	k8sClient, err := getK8sClient()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get kubernetes client")
	}

	secret, err := k8sClient.CoreV1().Secrets(cluster.Namespace).Get(context.Background(), fmt.Sprintf("%s-kubeconfig", cluster.Name), metav1.GetOptions{})
	if err != nil {
		return nil, microerror.Mask(err)
	}

	var kubeconfig []byte
	_, err = base64.StdEncoding.Decode(kubeconfig, secret.Data["value"])
	if err != nil {
		return nil, microerror.Mask(err)
	}

	var restConfig *rest.Config
	{
		c := k8srestconfig.Config{
			KubeConfig: string(kubeconfig),
		}

		restConfig, err = k8srestconfig.New(c)
		if err != nil {
			return nil, microerror.Mask(err)
		}
	}

	var clusterK8sClients k8sclient.Interface
	{
		c := k8sclient.ClientsConfig{
			RestConfig: restConfig,
		}

		clusterK8sClients, err = k8sclient.NewClients(c)
		if err != nil {
			return nil, microerror.Mask(err)
		}
	}

	return clusterK8sClients.K8sClient(), nil
}

func getK8sClient() (kubernetes.Interface, error) {
	var err error
	var restConfig *rest.Config
	{
		c := k8srestconfig.Config{
			InCluster: true,
		}

		restConfig, err = k8srestconfig.New(c)
		if err != nil {
			return nil, microerror.Mask(err)
		}
	}

	var k8sClients *k8sclient.Clients
	{
		c := k8sclient.ClientsConfig{
			RestConfig: restConfig,
		}

		k8sClients, err = k8sclient.NewClients(c)
		if err != nil {
			return nil, microerror.Mask(err)
		}

		return k8sClients.K8sClient(), nil
	}

}
