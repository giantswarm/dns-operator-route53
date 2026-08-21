//go:build integration

package controllers

import (
	"context"
	"fmt"
	"os"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	capi "sigs.k8s.io/cluster-api/api/core/v1beta2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/giantswarm/dns-operator-route53/internal/fakeroute53"
	"github.com/giantswarm/dns-operator-route53/internal/testenv"
	"github.com/giantswarm/dns-operator-route53/pkg/cloud/scope"
	"github.com/giantswarm/dns-operator-route53/pkg/cloud/services/route53"
	"github.com/giantswarm/dns-operator-route53/pkg/key"
)

const (
	baseDomain        = "test.gigantic.io"
	managementCluster = "testmc"

	infrastructureKind = "OpenStackCluster"
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

// fixture holds one reconciler with its own namespace and its own Route53 state.
type fixture struct {
	reconciler *ClusterReconciler
	route53    *fakeroute53.Fake

	namespace string
	name      string
}

// setup creates a fixture. The reconciler talks to the test API server and to the
// Route53 fake. It wraps the real cluster scope, and only replaces the workload
// cluster client, because the real one needs an in-cluster REST config.
func setup(t *testing.T) *fixture {
	t.Helper()

	ctx := context.Background()

	if err := testenv.ResetCache(); err != nil {
		t.Fatalf("failed to reset the cache: %v", err)
	}
	if err := env.CleanWorkloadServices(ctx); err != nil {
		t.Fatalf("failed to clean the workload cluster services: %v", err)
	}

	namespace, err := env.CreateNamespace(ctx)
	if err != nil {
		t.Fatalf("failed to create a namespace: %v", err)
	}

	fake := fakeroute53.New()
	fake.AddZone(baseDomain)

	reconciler := &ClusterReconciler{
		Client:            env.Client(),
		BaseDomain:        baseDomain,
		ManagementCluster: managementCluster,
		NewRoute53Service: func(clusterScope scope.Route53Scope) *route53.Service {
			return route53.NewServiceWithClient(&workloadClusterScope{
				Route53Scope: clusterScope,
				k8sClient:    env.Client(),
			}, fake)
		},
	}

	return &fixture{
		reconciler: reconciler,
		route53:    fake,
		namespace:  namespace,
		name:       "foo",
	}
}

// workloadClusterScope replaces the workload cluster client of the real scope.
// Every other method stays the real one, so the tests exercise the real base
// domain, cluster domain, bastion IP and annotation handling.
type workloadClusterScope struct {
	scope.Route53Scope

	k8sClient client.Client
}

func (s *workloadClusterScope) ClusterK8sClient(_ context.Context) (client.Client, error) {
	return s.k8sClient, nil
}

// createCluster creates a CAPI Cluster and, unless the kind says otherwise, the
// infrastructure cluster it points at.
func (f *fixture) createCluster(t *testing.T, options ...func(*capi.Cluster, *unstructured.Unstructured)) (*capi.Cluster, *unstructured.Unstructured) {
	t.Helper()

	ctx := context.Background()

	infraCluster := &unstructured.Unstructured{}
	infraCluster.SetGroupVersionKind(testenv.InfrastructureGroupVersion.WithKind(infrastructureKind))
	infraCluster.SetName(f.name)
	infraCluster.SetNamespace(f.namespace)

	cluster := &capi.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      f.name,
			Namespace: f.namespace,
		},
		Spec: capi.ClusterSpec{
			ControlPlaneEndpoint: capi.APIEndpoint{Host: "10.0.0.1", Port: 6443},
			InfrastructureRef: capi.ContractVersionedObjectReference{
				APIGroup: testenv.InfrastructureGroupVersion.Group,
				Kind:     infrastructureKind,
				Name:     f.name,
			},
		},
	}

	for _, option := range options {
		option(cluster, infraCluster)
	}

	if infraCluster.GetKind() == infrastructureKind {
		if err := env.Client().Create(ctx, infraCluster); err != nil {
			t.Fatalf("failed to create the infrastructure cluster: %v", err)
		}
		t.Cleanup(func() { f.removeInfrastructureCluster(t, infraCluster) })
	}

	if err := env.Client().Create(ctx, cluster); err != nil {
		t.Fatalf("failed to create the cluster: %v", err)
	}
	t.Cleanup(func() { f.removeCluster(t, cluster) })

	return cluster, infraCluster
}

// provisioned marks the cluster as provisioned, which is the phase the operator
// waits for.
func (f *fixture) provisioned(t *testing.T, cluster *capi.Cluster) {
	t.Helper()

	cluster.Status.Phase = string(capi.ClusterPhaseProvisioned)
	if err := env.Client().Status().Update(context.Background(), cluster); err != nil {
		t.Fatalf("failed to set the cluster phase: %v", err)
	}
}

// reconcile runs one reconcile loop for the cluster of the fixture.
func (f *fixture) reconcile(t *testing.T) (ctrl.Result, error) {
	t.Helper()

	return f.reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: f.namespace, Name: f.name},
	})
}

// deleteCluster marks the cluster for deletion. The finalizer keeps the object
// alive, so the deletion timestamp is set and the object stays readable.
func (f *fixture) deleteCluster(t *testing.T, cluster *capi.Cluster) {
	t.Helper()

	if err := env.Client().Delete(context.Background(), cluster); err != nil {
		t.Fatalf("failed to delete the cluster: %v", err)
	}
}

// get reads an object back from the API server.
func get(t *testing.T, object client.Object) {
	t.Helper()

	if err := env.Client().Get(context.Background(), client.ObjectKeyFromObject(object), object); err != nil {
		t.Fatalf("failed to read %s: %v", object.GetName(), err)
	}
}

// hasFinalizer reports whether the object carries the finalizer of the operator.
func hasFinalizer(object client.Object) bool {
	for _, finalizer := range object.GetFinalizers() {
		if finalizer == key.DNSFinalizerNameNew {
			return true
		}
	}
	return false
}

// removeCluster strips the finalizer and deletes the cluster. envtest runs no
// garbage collection, so a left over finalizer would keep the object forever.
func (f *fixture) removeCluster(t *testing.T, cluster *capi.Cluster) {
	t.Helper()

	ctx := context.Background()

	fresh := &capi.Cluster{}
	if err := env.Client().Get(ctx, client.ObjectKeyFromObject(cluster), fresh); err != nil {
		return
	}

	fresh.Finalizers = nil
	if err := env.Client().Update(ctx, fresh); err != nil {
		t.Logf("failed to strip the finalizers of cluster %s: %v", fresh.Name, err)
	}
	_ = env.Client().Delete(ctx, fresh)
}

func (f *fixture) removeInfrastructureCluster(t *testing.T, infraCluster *unstructured.Unstructured) {
	t.Helper()

	ctx := context.Background()

	fresh := &unstructured.Unstructured{}
	fresh.SetGroupVersionKind(infraCluster.GroupVersionKind())
	if err := env.Client().Get(ctx, client.ObjectKeyFromObject(infraCluster), fresh); err != nil {
		return
	}

	fresh.SetFinalizers(nil)
	if err := env.Client().Update(ctx, fresh); err != nil {
		t.Logf("failed to strip the finalizers of infrastructure cluster %s: %v", fresh.GetName(), err)
	}
	_ = env.Client().Delete(ctx, fresh)
}

// createIngressService builds a ready ingress controller Service on the stand-in
// workload cluster, so that a full reconcile writes ingress records.
func createIngressService(t *testing.T, ip string) {
	t.Helper()

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ingress-controller",
			Namespace: testenv.IngressNamespace,
			Labels:    map[string]string{"app.kubernetes.io/name": "ingress-nginx"},
		},
		Spec: corev1.ServiceSpec{
			Type:  corev1.ServiceTypeLoadBalancer,
			Ports: []corev1.ServicePort{{Port: 80}},
		},
	}

	ctx := context.Background()

	if err := env.Client().Create(ctx, service); err != nil {
		t.Fatalf("failed to create the ingress service: %v", err)
	}

	service.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{IP: ip}}
	if err := env.Client().Status().Update(ctx, service); err != nil {
		t.Fatalf("failed to set the status of the ingress service: %v", err)
	}
}

// paused marks the cluster as paused.
func paused(cluster *capi.Cluster, _ *unstructured.Unstructured) {
	cluster.Spec.Paused = ptr.To(true)
}

// pausedInfrastructure puts the paused annotation on the infrastructure cluster.
func pausedInfrastructure(_ *capi.Cluster, infraCluster *unstructured.Unstructured) {
	infraCluster.SetAnnotations(map[string]string{capi.PausedAnnotation: "true"})
}
