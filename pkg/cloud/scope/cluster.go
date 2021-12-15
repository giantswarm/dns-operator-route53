package scope

import (
	"context"
	"fmt"

	awsclient "github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/giantswarm/k8sclient/v6/pkg/k8sclient"
	"github.com/giantswarm/k8sclient/v6/pkg/k8srestconfig"
	"github.com/giantswarm/microerror"
	"github.com/giantswarm/micrologger"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/klog/klogr"
	capo "sigs.k8s.io/cluster-api-provider-openstack/api/v1alpha4"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	/* #nosec G101 */
	KubeConfigSecretSuffix = "-kubeconfig"
	KubeConfigSecretKey    = "value"
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
		return nil, microerror.Mask(err)
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
	k8sClient        client.Client
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
func (s *ClusterScope) ClusterK8sClient() client.Client {
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

func getClusterK8sClient(cluster *capo.OpenStackCluster) (client.Client, error) {
	newLogger, err := micrologger.New(micrologger.Config{})
	if err != nil {
		return nil, microerror.Mask(err)
	}

	kubeconfig, err := getClusterKubeConfig(cluster, newLogger)
	if err != nil {
		return nil, microerror.Mask(err)
	}

	config := k8srestconfig.Config{
		Logger:     newLogger,
		KubeConfig: kubeconfig,
	}

	return getK8sClient(config, newLogger)
}

func getClusterKubeConfig(cluster *capo.OpenStackCluster, logger micrologger.Logger) (string, error) {
	config := k8srestconfig.Config{
		Logger:    logger,
		InCluster: true,
	}

	k8sClient, err := getK8sClient(config, logger)
	if err != nil {
		return "", errors.Wrap(err, "failed to get kubernetes client")
	}

	var secret corev1.Secret

	o := client.ObjectKey{
		Name:      fmt.Sprintf("%s%s", cluster.Name, KubeConfigSecretSuffix),
		Namespace: cluster.Namespace,
	}

	if err := k8sClient.Get(context.Background(), o, &secret); err != nil {
		return "", microerror.Mask(err)
	}

	return string(secret.Data[KubeConfigSecretKey]), nil
}

func getK8sClient(config k8srestconfig.Config, logger micrologger.Logger) (client.Client, error) {
	var restConfig *rest.Config
	var err error
	{
		restConfig, err = k8srestconfig.New(config)
		if err != nil {
			return nil, microerror.Mask(err)
		}
	}

	var ctrlClient client.Client
	{
		c := k8sclient.ClientsConfig{
			Logger:     logger,
			RestConfig: restConfig,
		}

		k8sClients, err := k8sclient.NewClients(c)
		if err != nil {
			return nil, microerror.Mask(err)
		}

		ctrlClient = k8sClients.CtrlClient()
	}

	return ctrlClient, nil
}
