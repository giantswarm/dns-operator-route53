package scope

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	capi "sigs.k8s.io/cluster-api/api/core/v1beta2"

	"github.com/giantswarm/dns-operator-route53/pkg/key"
)

const testBaseDomain = "test.gigantic.io"

func newCluster(name string) *capi.Cluster {
	return &capi.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "org-test"},
	}
}

func newInfraCluster() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{}}
}

func TestNewClusterScopeRejectsAnInvalidConfig(t *testing.T) {
	for _, tc := range []struct {
		name   string
		params ClusterScopeParams
	}{
		{
			name: "an empty base domain",
			params: ClusterScopeParams{
				Cluster:               newCluster("foo"),
				InfrastructureCluster: newInfraCluster(),
			},
		},
		{
			name: "a missing infrastructure cluster",
			params: ClusterScopeParams{
				BaseDomain: testBaseDomain,
				Cluster:    newCluster("foo"),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewClusterScope(context.Background(), tc.params)
			if !IsInvalidConfig(err) {
				t.Errorf("error = %v, want an invalid config error", err)
			}
		})
	}
}

func TestNewClusterScopeAcceptsAValidConfig(t *testing.T) {
	// RoleArn stays empty on purpose. A role ARN makes the constructor call the
	// AWS security token service.
	clusterScope, err := NewClusterScope(context.Background(), ClusterScopeParams{
		BaseDomain:            testBaseDomain,
		Cluster:               newCluster("foo"),
		InfrastructureCluster: newInfraCluster(),
		ManagementCluster:     "testmc",
	})
	if err != nil {
		t.Fatalf("NewClusterScope: %v", err)
	}

	if got, want := clusterScope.Name(), "foo"; got != want {
		t.Errorf("Name = %s, want %s", got, want)
	}
	if got, want := clusterScope.BaseDomain(), testBaseDomain; got != want {
		t.Errorf("BaseDomain = %s, want %s", got, want)
	}
	if got, want := clusterScope.ClusterDomain(), "foo.test.gigantic.io"; got != want {
		t.Errorf("ClusterDomain = %s, want %s", got, want)
	}
	if got, want := clusterScope.ManagementCluster(), "testmc"; got != want {
		t.Errorf("ManagementCluster = %s, want %s", got, want)
	}
	if clusterScope.Session() == nil {
		t.Errorf("Session is nil")
	}
}

func TestClusterScopeAPIEndpoint(t *testing.T) {
	cluster := newCluster("foo")
	cluster.Spec.ControlPlaneEndpoint = capi.APIEndpoint{Host: "10.0.0.1", Port: 6443}

	clusterScope := &ClusterScope{cluster: cluster}

	if got, want := clusterScope.APIEndpoint(), "10.0.0.1"; got != want {
		t.Errorf("APIEndpoint = %s, want %s", got, want)
	}
}

func TestClusterScopeBastionIP(t *testing.T) {
	withFloatingIP := newInfraCluster()
	if err := unstructured.SetNestedField(withFloatingIP.Object, "10.0.0.9", "status", "bastion", "floatingIP"); err != nil {
		t.Fatalf("failed to build the infrastructure cluster: %v", err)
	}

	for _, tc := range []struct {
		name            string
		staticBastionIP string
		infraCluster    *unstructured.Unstructured
		want            string
	}{
		{
			name:            "the static IP wins",
			staticBastionIP: "10.0.0.8",
			infraCluster:    withFloatingIP,
			want:            "10.0.0.8",
		},
		{
			name:         "the floating IP of the infrastructure cluster",
			infraCluster: withFloatingIP,
			want:         "10.0.0.9",
		},
		{
			name:         "no bastion at all",
			infraCluster: newInfraCluster(),
			want:         "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clusterScope := &ClusterScope{
				cluster:         newCluster("foo"),
				infraCluster:    tc.infraCluster,
				staticBastionIP: tc.staticBastionIP,
			}

			if got := clusterScope.BastionIP(); got != tc.want {
				t.Errorf("BastionIP = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestClusterScopeWildcardCNAMETarget(t *testing.T) {
	for _, tc := range []struct {
		name       string
		annotation string
		want       string
	}{
		{
			name: "without the annotation",
			want: "",
		},
		{
			name:       "with an empty annotation",
			annotation: "",
			want:       "",
		},
		{
			name:       "with the annotation",
			annotation: "custom",
			want:       "custom.foo.test.gigantic.io",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cluster := newCluster("foo")
			if tc.annotation != "" {
				cluster.Annotations = map[string]string{key.AnnotationWildcardCNAMETarget: tc.annotation}
			}

			clusterScope := &ClusterScope{cluster: cluster, baseDomain: testBaseDomain}

			if got := clusterScope.WildcardCNAMETarget(); got != tc.want {
				t.Errorf("WildcardCNAMETarget = %s, want %s", got, tc.want)
			}
		})
	}
}
