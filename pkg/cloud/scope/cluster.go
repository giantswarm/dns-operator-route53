package scope

import (
	"context"
	"fmt"

	awsclient "github.com/aws/aws-sdk-go/aws/client"
	"github.com/giantswarm/k8sclient/v6/pkg/k8sclient"
	"github.com/giantswarm/k8sclient/v6/pkg/k8srestconfig"
	"github.com/giantswarm/microerror"
	"github.com/giantswarm/micrologger"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	capo "sigs.k8s.io/cluster-api-provider-openstack/api/v1alpha4"
	capi "sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/giantswarm/dns-operator-openstack/pkg/log"
)

const (
	/* #nosec G101 */
	KubeConfigSecretSuffix = "-kubeconfig"
	KubeConfigSecretKey    = "value"
)

// NewClusterScope creates a new Scope from the supplied parameters.
// This is meant to be called for each reconcile iteration.
func NewClusterScope(ctx context.Context, params ClusterScopeParams) (*ClusterScope, error) {
	if params.AWSSession == nil {
		return nil, microerror.Maskf(invalidConfigError, fmt.Sprintf("%T.AWSSession is required", params))
	}
	if params.ManagementClient == nil {
		return nil, microerror.Maskf(invalidConfigError, fmt.Sprintf("%T.ManagementClient is required", params))
	}
	if params.Logger == nil {
		return nil, microerror.Maskf(invalidConfigError, fmt.Sprintf("%T.Logger is required", params))
	}
	if params.BaseDomain == "" {
		return nil, microerror.Maskf(invalidConfigError, fmt.Sprintf("%T.BaseDomain is required", params))
	}
	if params.InfrastructureCluster == nil {
		return nil, microerror.Maskf(invalidConfigError, fmt.Sprintf("%T.InfrastructureCluster is required", params))
	}
	if params.ManagementCluster == "" {
		return nil, microerror.Maskf(invalidConfigError, fmt.Sprintf("%T.ManagementCluster is required", params))
	}

	return &ClusterScope{
		awsSession:       params.AWSSession,
		managementClient: params.ManagementClient,
		Logger:           params.Logger,

		baseDomain:        params.BaseDomain,
		infraCluster:      params.InfrastructureCluster,
		managementCluster: params.ManagementCluster,
	}, nil
}

// APIEndpoint returns the Openstack infrastructure Kubernetes API endpoint.
func (s *ClusterScope) APIEndpoint() string {
	return s.infraCluster.Spec.ControlPlaneEndpoint.Host
}

// BaseDomain returns the cluster basedomain.
func (s *ClusterScope) BaseDomain() string {
	return s.baseDomain
}

// BastionIP returns the bastion IP.
func (s *ClusterScope) BastionIP() string {
	if s.infraCluster.Status.Bastion != nil {
		return s.infraCluster.Status.Bastion.FloatingIP
	}

	return ""
}

// InfrastructureCluster returns the infrastructure cluster.
func (s *ClusterScope) InfrastructureCluster() *capo.OpenStackCluster {
	return s.infraCluster
}

// ManagementCluster returns the name of the management cluster.
func (s *ClusterScope) ManagementCluster() string {
	return s.managementCluster
}

// ClusterK8sClient returns a client to interact with the cluster.
func (s *ClusterScope) ClusterK8sClient(ctx context.Context) (client.Client, error) {
	if s.workloadClient == nil {
		var err error
		s.workloadClient, err = s.getClusterK8sClient(ctx)
		if err != nil {
			return nil, microerror.Mask(err)
		}
	}

	return s.workloadClient, nil
}

// ClusterDomain returns the cluster domain.
func (s *ClusterScope) ClusterDomain() string {
	return fmt.Sprintf("%s.%s", s.Name(), s.baseDomain)
}

// Name returns the Openstack cluster name.
func (s *ClusterScope) Name() string {
	return s.infraCluster.Labels[capi.ClusterLabelName]
}

// Session returns the AWS SDK session. Used for creating cluster client.
func (s *ClusterScope) Session() awsclient.ConfigProvider {
	return s.awsSession
}

type nopWriter struct{}

func (w nopWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func (s *ClusterScope) loggerAsMicrologger() (micrologger.Logger, error) {
	if adapter, ok := s.Logger.(log.Logger); ok {
		return adapter.Logger, nil
	}
	return micrologger.New(micrologger.Config{
		IOWriter: nopWriter{},
	})
}

func (s *ClusterScope) getClusterK8sClient(ctx context.Context) (client.Client, error) {
	kubeconfig, err := s.getClusterKubeConfig(ctx)
	if err != nil {
		return nil, microerror.Mask(err)
	}

	logger, err := s.loggerAsMicrologger()
	if err != nil {
		return nil, microerror.Mask(err)
	}

	config := k8srestconfig.Config{
		Logger:     logger,
		KubeConfig: kubeconfig,
	}

	var restConfig *rest.Config
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

func (s *ClusterScope) getClusterKubeConfig(ctx context.Context) (string, error) {
	var secret corev1.Secret
	o := client.ObjectKey{
		Name:      fmt.Sprintf("%s%s", s.Name(), KubeConfigSecretSuffix),
		Namespace: s.infraCluster.Namespace,
	}

	if err := s.managementClient.Get(ctx, o, &secret); err != nil {
		return "", microerror.Mask(err)
	}

	return string(secret.Data[KubeConfigSecretKey]), nil
}
