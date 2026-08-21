//go:build integration

package route53

import (
	"context"
	"fmt"
	"os"
	"testing"

	awsclient "github.com/aws/aws-sdk-go/aws/client"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	capi "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/giantswarm/dns-operator-route53/internal/fakeroute53"
	"github.com/giantswarm/dns-operator-route53/internal/testenv"
)

const (
	baseDomain  = "test.gigantic.io"
	clusterName = "foo"
)

var env *testenv.Env

func TestMain(m *testing.M) {
	var err error

	env, err = testenv.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start the test environment: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	if err := env.Stop(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to stop the test environment: %v\n", err)
	}

	os.Exit(code)
}

// fixture is one test case with its own cache, its own Route53 state and a clean
// set of workload cluster Services.
type fixture struct {
	service *Service
	scope   *fakeScope
	route53 *fakeroute53.Fake

	// baseZoneID is the hosted zone of the base domain. The operator writes the
	// NS delegation of every cluster into it.
	baseZoneID string
}

// setup prepares a fixture. The base hosted zone exists, the cluster hosted zone
// does not.
func setup(t *testing.T) *fixture {
	t.Helper()

	if err := testenv.ResetCache(); err != nil {
		t.Fatalf("failed to reset the cache: %v", err)
	}

	if err := env.CleanWorkloadServices(context.Background()); err != nil {
		t.Fatalf("failed to clean the workload cluster services: %v", err)
	}

	fake := fakeroute53.New()
	baseZoneID := fake.AddZone(baseDomain)

	scope := &fakeScope{
		apiEndpoint:       "10.0.0.1",
		baseDomain:        baseDomain,
		name:              clusterName,
		managementCluster: "testmc",
		k8sClient:         env.Client(),
		cluster: &capi.Cluster{
			ObjectMeta: metav1.ObjectMeta{Name: clusterName, Namespace: "org-test"},
		},
	}

	return &fixture{
		service:    NewServiceWithClient(scope, fake),
		scope:      scope,
		route53:    fake,
		baseZoneID: baseZoneID,
	}
}

// clusterDomain returns the DNS domain of the test cluster.
func clusterDomain() string {
	return fmt.Sprintf("%s.%s", clusterName, baseDomain)
}

// createService creates a Service on the stand-in workload cluster and removes it
// when the test ends.
func createService(t *testing.T, service *corev1.Service) {
	t.Helper()

	ctx := context.Background()

	// Create drops the status, so keep it and write it back afterwards.
	status := service.Status

	if err := env.Client().Create(ctx, service); err != nil {
		t.Fatalf("failed to create service %s: %v", service.Name, err)
	}

	// The load balancer address lives in the status, which is a subresource.
	if len(status.LoadBalancer.Ingress) > 0 {
		service.Status = status
		if err := env.Client().Status().Update(ctx, service); err != nil {
			t.Fatalf("failed to set the status of service %s: %v", service.Name, err)
		}
	}
}

// newIngressService builds an ingress controller Service with a load balancer IP.
func newIngressService(name, appName, ip string) *corev1.Service {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testenv.IngressNamespace,
			Labels:    map[string]string{"app.kubernetes.io/name": appName},
		},
		Spec: corev1.ServiceSpec{
			Type:  corev1.ServiceTypeLoadBalancer,
			Ports: []corev1.ServicePort{{Port: 80}},
		},
	}

	if ip != "" {
		service.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{IP: ip}}
	}

	return service
}

// newGatewayService builds a gateway Service which the operator manages.
func newGatewayService(name, hostname, ip string) *corev1.Service {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testenv.GatewayNamespace,
			Annotations: map[string]string{
				"giantswarm.io/external-dns":                "managed",
				"external-dns.alpha.kubernetes.io/hostname": hostname,
			},
		},
		Spec: corev1.ServiceSpec{
			Type:  corev1.ServiceTypeLoadBalancer,
			Ports: []corev1.ServicePort{{Port: 80}},
		},
	}

	if ip != "" {
		service.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{IP: ip}}
	}

	return service
}

// fakeScope implements scope.Route53Scope. It hands the operator the test API
// server as the workload cluster client, which avoids the in-cluster REST config
// and the kubeconfig secret of the real scope.
type fakeScope struct {
	apiEndpoint         string
	baseDomain          string
	bastionIP           string
	cluster             *capi.Cluster
	k8sClient           client.Client
	managementCluster   string
	name                string
	wildcardCNAMETarget string
}

func (s *fakeScope) APIEndpoint() string       { return s.apiEndpoint }
func (s *fakeScope) BaseDomain() string        { return s.baseDomain }
func (s *fakeScope) BastionIP() string         { return s.bastionIP }
func (s *fakeScope) Cluster() *capi.Cluster    { return s.cluster }
func (s *fakeScope) ManagementCluster() string { return s.managementCluster }
func (s *fakeScope) Name() string              { return s.name }

func (s *fakeScope) ClusterDomain() string {
	return fmt.Sprintf("%s.%s", s.name, s.baseDomain)
}

func (s *fakeScope) ClusterK8sClient(_ context.Context) (client.Client, error) {
	return s.k8sClient, nil
}

func (s *fakeScope) InfrastructureCluster() *unstructured.Unstructured { return nil }

func (s *fakeScope) Session() awsclient.ConfigProvider { return nil }

func (s *fakeScope) WildcardCNAMETarget() string {
	if s.wildcardCNAMETarget == "" {
		return ""
	}
	return fmt.Sprintf("%s.%s", s.wildcardCNAMETarget, s.ClusterDomain())
}
