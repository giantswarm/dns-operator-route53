// Package testenv starts a real Kubernetes API server for the local integration
// tests.
//
// One API server serves two roles. It is the management cluster, which holds the
// CAPI Cluster and the infrastructure cluster, and it stands in for the workload
// cluster, which holds the ingress and gateway Services. A real API server is
// necessary because the operator sends its ingress label selector through the raw
// list options, which the controller-runtime fake client ignores.
package testenv

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	capi "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	dnscache "github.com/giantswarm/dns-operator-route53/pkg/cloud/cache"
)

// GatewayNamespace is the namespace which the operator watches for gateway
// Services. The operator hardcodes it, so tests cannot isolate it per test.
const GatewayNamespace = "envoy-gateway-system"

// IngressNamespace is the namespace which the operator watches for ingress
// Services. The operator hardcodes it too.
const IngressNamespace = "kube-system"

// InfrastructureGroupVersion is the group and version of the infrastructure
// cluster CRD which the tests install. It must agree with the contract label of
// test/crds/infrastructure.cluster.x-k8s.io_openstackclusters.yaml.
var InfrastructureGroupVersion = schema.GroupVersion{
	Group:   "infrastructure.cluster.x-k8s.io",
	Version: "v1beta1",
}

// Env is a running API server.
type Env struct {
	environment *envtest.Environment
	client      client.Client
	config      *rest.Config

	namespaces int
}

// New starts the API server and installs the CRDs from test/crds.
func New() (*Env, error) {
	crdDir, err := crdDirectory()
	if err != nil {
		return nil, err
	}

	// The operator builds an AWS session even when it never calls AWS. Keep it
	// away from the credentials of the machine which runs the tests.
	for name, value := range map[string]string{
		"AWS_ACCESS_KEY_ID":         "integration-test",
		"AWS_SECRET_ACCESS_KEY":     "integration-test",
		"AWS_REGION":                "eu-central-1",
		"AWS_EC2_METADATA_DISABLED": "true",
	} {
		if err := os.Setenv(name, value); err != nil {
			return nil, err
		}
	}

	environment := &envtest.Environment{
		CRDDirectoryPaths:     []string{crdDir},
		ErrorIfCRDPathMissing: true,
	}

	config, err := environment.Start()
	if err != nil {
		return nil, fmt.Errorf("failed to start envtest: %w", err)
	}

	scheme := clientgoscheme.Scheme
	if err := capi.AddToScheme(scheme); err != nil {
		return nil, err
	}
	if err := apiextensionsv1.AddToScheme(scheme); err != nil {
		return nil, err
	}

	// A direct client, so the tests see every write immediately.
	ctrlClient, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}

	env := &Env{
		environment: environment,
		client:      ctrlClient,
		config:      config,
	}

	if err := env.createNamespace(context.Background(), GatewayNamespace); err != nil {
		_ = environment.Stop()
		return nil, err
	}

	return env, nil
}

// Stop shuts the API server down.
func (e *Env) Stop() error {
	return e.environment.Stop()
}

// Client returns a client for the API server.
func (e *Env) Client() client.Client {
	return e.client
}

// Config returns the REST config of the API server.
func (e *Env) Config() *rest.Config {
	return e.config
}

// CreateNamespace creates a namespace with a unique name and returns it.
func (e *Env) CreateNamespace(ctx context.Context) (string, error) {
	e.namespaces++
	name := fmt.Sprintf("test-%03d", e.namespaces)

	if err := e.createNamespace(ctx, name); err != nil {
		return "", err
	}

	return name, nil
}

func (e *Env) createNamespace(ctx context.Context, name string) error {
	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}

	if err := e.client.Create(ctx, namespace); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}

	return nil
}

// CleanWorkloadServices deletes every Service from the two namespaces which the
// operator watches on the workload cluster. Their names are hardcoded in the
// operator, so tests have to share them and must clean up after themselves.
func (e *Env) CleanWorkloadServices(ctx context.Context) error {
	for _, namespace := range []string{IngressNamespace, GatewayNamespace} {
		err := e.client.DeleteAllOf(ctx, &corev1.Service{}, client.InNamespace(namespace))
		if err != nil {
			return err
		}
	}

	return nil
}

// ResetCache empties the operator cache, and creates it on the first call. The
// cache is a package global with a six minute lifetime, and the operator writes
// it before it calls AWS, so a leaked entry would suppress writes in a later
// test. The cache reserves a lot of memory, so tests share one instance instead
// of building a new one each time.
func ResetCache() error {
	if dnscache.DNSOperatorCache == nil {
		cache, err := dnscache.NewDNSOperatorCache()
		if err != nil {
			return err
		}
		dnscache.DNSOperatorCache = cache

		return nil
	}

	return dnscache.DNSOperatorCache.Reset()
}

// crdDirectory returns the test/crds directory and checks that the downloaded
// CAPI CRD is in it.
func crdDirectory() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("failed to determine the location of the test environment")
	}

	dir := filepath.Join(filepath.Dir(file), "..", "..", "test", "crds")

	capiCRD := filepath.Join(dir, "cluster.x-k8s.io_clusters.yaml")
	if _, err := os.Stat(capiCRD); err != nil {
		return "", fmt.Errorf("%s is missing. Run 'make test-integration' to download it: %w", capiCRD, err)
	}

	return dir, nil
}
