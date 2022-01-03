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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/klog/klogr"
	capo "sigs.k8s.io/cluster-api-provider-openstack/api/v1alpha4"
	capi "sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	/* #nosec G101 */
	KubeConfigSecretSuffix = "-kubeconfig"
	KubeConfigSecretKey    = "value"
)

// ClusterScopeParams defines the input parameters used to create a new Scope.
type ClusterScopeParams struct {
	CoreCluster           *capi.Cluster
	InfrastructureCluster *capo.OpenStackCluster
	BaseDomain            string
	Logger                logr.Logger
}

// NewClusterScope creates a new Scope from the supplied parameters.
// This is meant to be called for each reconcile iteration.
func NewClusterScope(ctx context.Context, params ClusterScopeParams) (*ClusterScope, error) {
	if params.CoreCluster == nil {
		return nil, microerror.Maskf(invalidConfigError, "failed to generate new scope from nil CoreCluster")
	}
	if params.InfrastructureCluster == nil {
		return nil, microerror.Maskf(invalidConfigError, "failed to generate new scope from nil InfrastructureCluster")
	}
	if params.BaseDomain == "" {
		return nil, microerror.Maskf(invalidConfigError, "failed to generate new scope from empty BaseDomain")
	}
	if params.Logger == nil {
		params.Logger = klogr.New()
	}

	awsSession, err := session.NewSession()
	if err != nil {
		return nil, microerror.Mask(err)
	}

	clusterK8sClient, err := getClusterK8sClient(ctx, params.CoreCluster)
	if err != nil {
		return nil, microerror.Mask(err)
	}

	return &ClusterScope{
		coreCluster:  params.CoreCluster,
		infraCluster: params.InfrastructureCluster,
		baseDomain:   params.BaseDomain,
		k8sClient:    clusterK8sClient,
		Logger:       params.Logger,
		session:      awsSession,
	}, nil
}

// ClusterScope defines the basic context for an actuator to operate upon.
type ClusterScope struct {
	coreCluster  *capi.Cluster
	infraCluster *capo.OpenStackCluster
	baseDomain   string
	k8sClient    client.Client
	logr.Logger
	session awsclient.ConfigProvider
}

// APIEndpoint returns the AWS infrastructure Kubernetes API endpoint.
func (s *ClusterScope) APIEndpoint() string {
	return s.infraCluster.Spec.ControlPlaneEndpoint.Host
}

// BaseDomain returns the cluster basedomain.
func (s *ClusterScope) BaseDomain() string {
	return s.baseDomain
}

// CoreCluster returns the core cluster.
func (s *ClusterScope) CoreCluster() *capi.Cluster {
	return s.coreCluster
}

// InfrastructureCluster returns the infrastructure cluster.
func (s *ClusterScope) InfrastructureCluster() *capo.OpenStackCluster {
	return s.infraCluster
}

// ClusterK8sClient returns a client to interact with the cluster.
func (s *ClusterScope) ClusterK8sClient() client.Client {
	return s.k8sClient
}

// Name returns the AWS infrastructure cluster name.
func (s *ClusterScope) Name() string {
	return s.infraCluster.Name
}

// Session returns the AWS SDK session. Used for creating cluster client.
func (s *ClusterScope) Session() awsclient.ConfigProvider {
	return s.session
}

func getClusterK8sClient(ctx context.Context, cluster *capi.Cluster) (client.Client, error) {
	newLogger, err := micrologger.New(micrologger.Config{})
	if err != nil {
		return nil, microerror.Mask(err)
	}

	kubeconfig, err := getClusterKubeConfig(ctx, cluster, newLogger)
	if err != nil {
		return nil, microerror.Mask(err)
	}

	config := k8srestconfig.Config{
		Logger:     newLogger,
		KubeConfig: kubeconfig,
	}

	return getK8sClient(config, newLogger)
}

func getClusterKubeConfig(ctx context.Context, cluster *capi.Cluster, logger micrologger.Logger) (string, error) {
	config := k8srestconfig.Config{
		Logger:    logger,
		InCluster: true,
	}

	k8sClient, err := getK8sClient(config, logger)
	if err != nil {
		return "", microerror.Mask(err)
	}

	var secret corev1.Secret

	o := client.ObjectKey{
		Name:      fmt.Sprintf("%s%s", cluster.Name, KubeConfigSecretSuffix),
		Namespace: cluster.Namespace,
	}

	if err := k8sClient.Get(ctx, o, &secret); err != nil {
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
