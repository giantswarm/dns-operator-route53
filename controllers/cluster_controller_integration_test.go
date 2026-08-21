//go:build integration

package controllers

import (
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	capi "sigs.k8s.io/cluster-api/api/core/v1beta2"

	"github.com/giantswarm/dns-operator-route53/internal/fakeroute53"
	"github.com/giantswarm/dns-operator-route53/pkg/cloud/services/route53"
)

// The AWS provider is switched off, so the operator leaves those clusters alone.
func TestReconcileSkipsADisabledInfrastructureProvider(t *testing.T) {
	f := setup(t)

	cluster, _ := f.createCluster(t, func(cluster *capi.Cluster, infraCluster *unstructured.Unstructured) {
		cluster.Spec.InfrastructureRef.Kind = "AWSCluster"
		infraCluster.SetKind("AWSCluster")
	})

	result, err := f.reconcile(t)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Errorf("RequeueAfter = %v, want 0", result.RequeueAfter)
	}

	get(t, cluster)
	if hasFinalizer(cluster) {
		t.Errorf("the cluster got a finalizer although the provider is disabled")
	}
	if len(f.route53.Calls()) != 0 {
		t.Errorf("AWS calls = %v, want none", f.route53.Calls())
	}
}

func TestReconcileSkipsAPausedCluster(t *testing.T) {
	f := setup(t)
	cluster, _ := f.createCluster(t, paused)

	result, err := f.reconcile(t)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Errorf("RequeueAfter = %v, want 0", result.RequeueAfter)
	}

	get(t, cluster)
	if hasFinalizer(cluster) {
		t.Errorf("a paused cluster got a finalizer")
	}
}

func TestReconcileSkipsAPausedInfrastructureCluster(t *testing.T) {
	f := setup(t)
	cluster, _ := f.createCluster(t, pausedInfrastructure)

	result, err := f.reconcile(t)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Errorf("RequeueAfter = %v, want 0", result.RequeueAfter)
	}

	get(t, cluster)
	if hasFinalizer(cluster) {
		t.Errorf("the cluster got a finalizer although the infrastructure cluster is paused")
	}
}

func TestReconcileFailsWithoutTheInfrastructureCluster(t *testing.T) {
	f := setup(t)

	f.createCluster(t, func(cluster *capi.Cluster, infraCluster *unstructured.Unstructured) {
		// An empty kind stops the fixture from creating the object, while the
		// cluster still points at it.
		infraCluster.SetKind("")
	})

	if _, err := f.reconcile(t); err == nil {
		t.Fatalf("Reconcile returned no error although the infrastructure cluster is missing")
	}
}

// The operator sets its finalizer immediately, but it waits for the provisioned
// phase before it touches DNS.
func TestReconcileWaitsForTheProvisionedPhase(t *testing.T) {
	f := setup(t)
	cluster, infraCluster := f.createCluster(t)

	result, err := f.reconcile(t)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if result.RequeueAfter != 2*time.Minute {
		t.Errorf("RequeueAfter = %v, want 2m", result.RequeueAfter)
	}

	get(t, cluster)
	if !hasFinalizer(cluster) {
		t.Errorf("the cluster has no finalizer")
	}

	get(t, infraCluster)
	if !hasFinalizer(infraCluster) {
		t.Errorf("the infrastructure cluster has no finalizer")
	}

	if len(f.route53.Calls()) != 0 {
		t.Errorf("AWS calls = %v, want none before the cluster is provisioned", f.route53.Calls())
	}
}

func TestReconcileCreatesTheDNSRecordsOfAProvisionedCluster(t *testing.T) {
	f := setup(t)
	cluster, _ := f.createCluster(t)
	f.provisioned(t, cluster)
	createIngressService(t, "10.0.0.2")

	result, err := f.reconcile(t)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if result.RequeueAfter != time.Minute {
		t.Errorf("RequeueAfter = %v, want 1m", result.RequeueAfter)
	}

	clusterDomain := f.name + "." + baseDomain

	zoneID := ""
	for _, id := range f.route53.ZoneIDs() {
		if _, ok := f.route53.Record(id, "api."+clusterDomain, "A"); ok {
			zoneID = id
		}
	}
	if zoneID == "" {
		t.Fatalf("no hosted zone holds the api record of %s", clusterDomain)
	}

	for _, want := range []struct {
		name       string
		recordType string
		value      string
	}{
		{"api." + clusterDomain, "A", "10.0.0.1"},
		{"ingress." + clusterDomain, "A", "10.0.0.2"},
		{"*." + clusterDomain, "CNAME", "ingress." + clusterDomain},
	} {
		record, ok := f.route53.Record(zoneID, want.name, want.recordType)
		if !ok {
			t.Errorf("record %s %s is missing", want.name, want.recordType)
			continue
		}
		if record.Values[0] != want.value {
			t.Errorf("record %s %s = %s, want %s", want.name, want.recordType, record.Values[0], want.value)
		}
	}
}

// The operator reads the bastion IP from the infrastructure cluster status. The
// API record has to be correct already, because the operator decides between the
// two records in one if/else chain.
func TestReconcileUsesTheBastionIPOfTheInfrastructureCluster(t *testing.T) {
	f := setup(t)
	cluster, infraCluster := f.createCluster(t)
	f.provisioned(t, cluster)

	get(t, infraCluster)
	if err := unstructured.SetNestedField(infraCluster.Object, "10.0.0.9", "status", "bastion", "floatingIP"); err != nil {
		t.Fatalf("failed to set the bastion IP: %v", err)
	}
	if err := env.Client().Status().Update(t.Context(), infraCluster); err != nil {
		t.Fatalf("failed to update the infrastructure cluster status: %v", err)
	}

	// The first pass writes the api record, the second one the bastion record.
	for pass := 0; pass < 2; pass++ {
		if _, err := f.reconcile(t); err != nil {
			t.Fatalf("Reconcile pass %d: %v", pass+1, err)
		}
	}

	clusterDomain := f.name + "." + baseDomain

	for _, id := range f.route53.ZoneIDs() {
		if record, ok := f.route53.Record(id, "bastion1."+clusterDomain, "A"); ok {
			if record.Values[0] != "10.0.0.9" {
				t.Errorf("bastion record = %s, want 10.0.0.9", record.Values[0])
			}
			return
		}
	}

	t.Errorf("no hosted zone holds the bastion record of %s", clusterDomain)
}

func TestReconcileReportsAnIngressWhichIsNotReady(t *testing.T) {
	f := setup(t)
	cluster, _ := f.createCluster(t)
	f.provisioned(t, cluster)
	createIngressService(t, "")

	_, err := f.reconcile(t)
	if !route53.IsIngressNotReady(err) {
		t.Fatalf("error = %v, want an ingress not ready error", err)
	}
}

func TestReconcileDeletesTheDNSRecordsOfADeletedCluster(t *testing.T) {
	f := setup(t)
	cluster, infraCluster := f.createCluster(t)
	f.provisioned(t, cluster)

	if _, err := f.reconcile(t); err != nil {
		t.Fatalf("first Reconcile: %v", err)
	}

	get(t, cluster)
	f.deleteCluster(t, cluster)

	result, err := f.reconcile(t)
	if err != nil {
		t.Fatalf("second Reconcile: %v", err)
	}
	if result.RequeueAfter != 5*time.Minute {
		t.Errorf("RequeueAfter = %v, want 5m", result.RequeueAfter)
	}

	if got := f.route53.CallsTo(fakeroute53.OpDeleteHostedZone); got != 1 {
		t.Errorf("DeleteHostedZone calls = %d, want 1", got)
	}

	get(t, infraCluster)
	if hasFinalizer(infraCluster) {
		t.Errorf("the infrastructure cluster still has the finalizer")
	}
}

// A deleted cluster without the finalizer is already done, so the operator must
// not call AWS again.
func TestReconcileSkipsADeletedClusterWithoutTheFinalizer(t *testing.T) {
	f := setup(t)
	cluster, _ := f.createCluster(t, func(cluster *capi.Cluster, infraCluster *unstructured.Unstructured) {
		// A finalizer of another controller keeps the objects alive without
		// giving this operator anything to do.
		cluster.Finalizers = []string{"other.giantswarm.io/finalizer"}
		infraCluster.SetFinalizers([]string{"other.giantswarm.io/finalizer"})
	})

	f.deleteCluster(t, cluster)

	result, err := f.reconcile(t)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Errorf("RequeueAfter = %v, want 0", result.RequeueAfter)
	}
	if len(f.route53.Calls()) != 0 {
		t.Errorf("AWS calls = %v, want none", f.route53.Calls())
	}
}

func TestReconcileIgnoresAClusterWhichDoesNotExist(t *testing.T) {
	f := setup(t)

	result, err := f.reconcile(t)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Errorf("RequeueAfter = %v, want 0", result.RequeueAfter)
	}
}
