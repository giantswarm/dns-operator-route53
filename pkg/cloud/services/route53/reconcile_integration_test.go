//go:build integration

package route53

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"

	"github.com/giantswarm/dns-operator-route53/internal/fakeroute53"
)

func TestReconcileCreatesTheClusterHostedZone(t *testing.T) {
	f := setup(t)

	if err := f.service.ReconcileRoute53(context.Background()); err != nil {
		t.Fatalf("ReconcileRoute53: %v", err)
	}

	if got := f.route53.CallsTo(fakeroute53.OpCreateHostedZone); got != 1 {
		t.Errorf("CreateHostedZone calls = %d, want 1", got)
	}

	zoneID := f.clusterZoneID(t)
	record, ok := f.route53.Record(zoneID, clusterDomain(), route53.RRTypeNs)
	if !ok {
		t.Fatalf("the new hosted zone has no apex NS record")
	}
	if len(record.Values) == 0 {
		t.Errorf("the apex NS record has no values")
	}
}

func TestReconcileUsesAnExistingClusterHostedZone(t *testing.T) {
	f := setup(t)
	existing := f.route53.AddZone(clusterDomain())

	if err := f.service.ReconcileRoute53(context.Background()); err != nil {
		t.Fatalf("ReconcileRoute53: %v", err)
	}

	if got := f.route53.CallsTo(fakeroute53.OpCreateHostedZone); got != 0 {
		t.Errorf("CreateHostedZone calls = %d, want 0", got)
	}
	if got := f.route53.ZoneIDs(); len(got) != 2 {
		t.Errorf("hosted zones = %v, want the base zone and the cluster zone", got)
	}
	if _, ok := f.route53.Record(existing, "api."+clusterDomain(), route53.RRTypeA); !ok {
		t.Errorf("the existing hosted zone has no api record")
	}
}

// A hosted zone whose name only shares a prefix with the cluster domain is not
// the cluster hosted zone. The operator reads the first zone of the listing and
// compares the name, so this case has to create a new zone.
func TestReconcileIgnoresAPrefixNeighbourHostedZone(t *testing.T) {
	f := setup(t)
	neighbour := f.route53.AddZone("foobar." + baseDomain)

	if err := f.service.ReconcileRoute53(context.Background()); err != nil {
		t.Fatalf("ReconcileRoute53: %v", err)
	}

	if got := f.route53.CallsTo(fakeroute53.OpCreateHostedZone); got != 1 {
		t.Errorf("CreateHostedZone calls = %d, want 1", got)
	}
	if _, ok := f.route53.Record(neighbour, "api."+clusterDomain(), route53.RRTypeA); ok {
		t.Errorf("the neighbour hosted zone must not hold the cluster records")
	}
}

func TestReconcileServesTheHostedZoneIDFromTheCache(t *testing.T) {
	f := setup(t)

	if err := f.service.ReconcileRoute53(context.Background()); err != nil {
		t.Fatalf("first ReconcileRoute53: %v", err)
	}

	f.route53.ResetCalls()

	if err := f.service.ReconcileRoute53(context.Background()); err != nil {
		t.Fatalf("second ReconcileRoute53: %v", err)
	}

	if got := f.route53.CallsTo(fakeroute53.OpListHostedZonesByName); got != 0 {
		t.Errorf("ListHostedZonesByName calls = %d, want 0", got)
	}
}

func TestReconcileDelegatesTheClusterZoneFromTheBaseZone(t *testing.T) {
	f := setup(t)

	if err := f.service.ReconcileRoute53(context.Background()); err != nil {
		t.Fatalf("ReconcileRoute53: %v", err)
	}

	delegation, ok := f.route53.Record(f.baseZoneID, clusterDomain(), route53.RRTypeNs)
	if !ok {
		t.Fatalf("the base hosted zone has no NS delegation for %s", clusterDomain())
	}
	if delegation.TTL != 300 {
		t.Errorf("delegation TTL = %d, want 300", delegation.TTL)
	}

	apex, _ := f.route53.Record(f.clusterZoneID(t), clusterDomain(), route53.RRTypeNs)
	if diff := cmp.Diff(apex.Values, delegation.Values); diff != "" {
		t.Errorf("the delegation does not carry the name servers of the cluster zone (-apex +delegation):\n%s", diff)
	}
}

func TestReconcileWritesTheDelegationOnlyOnce(t *testing.T) {
	f := setup(t)

	if err := f.service.ReconcileRoute53(context.Background()); err != nil {
		t.Fatalf("first ReconcileRoute53: %v", err)
	}

	f.route53.ResetCalls()

	if err := f.service.ReconcileRoute53(context.Background()); err != nil {
		t.Fatalf("second ReconcileRoute53: %v", err)
	}

	if got := f.route53.CallsTo(fakeroute53.OpChangeResourceRecordSet); got != 0 {
		t.Errorf("ChangeResourceRecordSets calls = %d, want 0, because nothing changed", got)
	}
}

func TestReconcileFailsWhenTheBaseHostedZoneIsMissing(t *testing.T) {
	f := setup(t)
	f.route53 = fakeroute53.New()
	f.service.Route53Client = f.route53

	err := f.service.ReconcileRoute53(context.Background())
	if !IsHostedZoneNotFound(err) {
		t.Fatalf("error = %v, want a hosted zone not found error", err)
	}
}

func TestReconcileFailsWhileTheAPIEndpointIsEmpty(t *testing.T) {
	f := setup(t)
	f.scope.apiEndpoint = ""

	err := f.service.ReconcileRoute53(context.Background())
	if !errors.Is(err, aws.ErrMissingEndpoint) {
		t.Fatalf("error = %v, want %v", err, aws.ErrMissingEndpoint)
	}
}

func TestReconcileCreatesTheAPIRecord(t *testing.T) {
	f := setup(t)

	if err := f.service.ReconcileRoute53(context.Background()); err != nil {
		t.Fatalf("ReconcileRoute53: %v", err)
	}

	want := fakeroute53.Record{
		Name:   "api." + clusterDomain() + ".",
		Type:   route53.RRTypeA,
		TTL:    300,
		Values: []string{"10.0.0.1"},
	}

	got, ok := f.route53.Record(f.clusterZoneID(t), "api."+clusterDomain(), route53.RRTypeA)
	if !ok {
		t.Fatalf("the cluster hosted zone has no api record")
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("api record mismatch (-want +got):\n%s", diff)
	}
}

func TestReconcileLeavesACorrectAPIRecordAlone(t *testing.T) {
	f := setup(t)
	zoneID := f.route53.AddZone(clusterDomain())
	f.route53.AddRecord(zoneID, "api."+clusterDomain(), route53.RRTypeA, 300, "10.0.0.1")

	if err := f.service.ReconcileRoute53(context.Background()); err != nil {
		t.Fatalf("ReconcileRoute53: %v", err)
	}

	// Two changes are expected: the NS delegation in the base zone, and nothing
	// for the cluster records themselves.
	for _, record := range f.route53.Records(zoneID) {
		if record.Name == "api."+clusterDomain()+"." && record.Values[0] != "10.0.0.1" {
			t.Errorf("the api record changed to %v", record.Values)
		}
	}
	if got := f.route53.CallsTo(fakeroute53.OpChangeResourceRecordSet); got != 1 {
		t.Errorf("ChangeResourceRecordSets calls = %d, want 1 for the delegation only", got)
	}
}

// The operator decides between the API record and the bastion record in one
// if/else chain, so it only reconciles the bastion record while the API record
// needs no update.
func TestReconcileCreatesTheBastionRecord(t *testing.T) {
	f := setup(t)
	f.scope.bastionIP = "10.0.0.9"

	zoneID := f.route53.AddZone(clusterDomain())
	f.route53.AddRecord(zoneID, "api."+clusterDomain(), route53.RRTypeA, 300, "10.0.0.1")

	if err := f.service.ReconcileRoute53(context.Background()); err != nil {
		t.Fatalf("ReconcileRoute53: %v", err)
	}

	want := fakeroute53.Record{
		Name:   "bastion1." + clusterDomain() + ".",
		Type:   route53.RRTypeA,
		TTL:    300,
		Values: []string{"10.0.0.9"},
	}

	got, ok := f.route53.Record(zoneID, "bastion1."+clusterDomain(), route53.RRTypeA)
	if !ok {
		t.Fatalf("the cluster hosted zone has no bastion record")
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("bastion record mismatch (-want +got):\n%s", diff)
	}
}

func TestReconcileDeletesAnOrphanedBastionRecord(t *testing.T) {
	f := setup(t)

	zoneID := f.route53.AddZone(clusterDomain())
	f.route53.AddRecord(zoneID, "api."+clusterDomain(), route53.RRTypeA, 300, "10.0.0.1")
	f.route53.AddRecord(zoneID, "bastion1."+clusterDomain(), route53.RRTypeA, 300, "10.0.0.9")

	if err := f.service.ReconcileRoute53(context.Background()); err != nil {
		t.Fatalf("ReconcileRoute53: %v", err)
	}

	if _, ok := f.route53.Record(zoneID, "bastion1."+clusterDomain(), route53.RRTypeA); ok {
		t.Errorf("the orphaned bastion record is still there")
	}
}

func TestReconcileWithoutAnIngressServiceWritesNoIngressRecords(t *testing.T) {
	f := setup(t)

	if err := f.service.ReconcileRoute53(context.Background()); err != nil {
		t.Fatalf("ReconcileRoute53: %v", err)
	}

	if _, ok := f.route53.Record(f.clusterZoneID(t), "ingress."+clusterDomain(), route53.RRTypeA); ok {
		t.Errorf("an ingress record was written without an ingress service")
	}
}

func TestReconcileCreatesTheIngressRecords(t *testing.T) {
	for _, appName := range []string{"ingress-nginx", "nginx-ingress-controller"} {
		t.Run(appName, func(t *testing.T) {
			f := setup(t)
			createService(t, newIngressService("ingress-controller", appName, "10.0.0.2"))

			if err := f.service.ReconcileRoute53(context.Background()); err != nil {
				t.Fatalf("ReconcileRoute53: %v", err)
			}

			zoneID := f.clusterZoneID(t)

			want := []fakeroute53.Record{
				{
					Name:   "*." + clusterDomain() + ".",
					Type:   route53.RRTypeCname,
					TTL:    300,
					Values: []string{"ingress." + clusterDomain()},
				},
				{
					Name:   "ingress." + clusterDomain() + ".",
					Type:   route53.RRTypeA,
					TTL:    300,
					Values: []string{"10.0.0.2"},
				},
			}

			var got []fakeroute53.Record
			for _, record := range f.route53.Records(zoneID) {
				if record.Type == route53.RRTypeA && record.Name == "ingress."+clusterDomain()+"." {
					got = append(got, record)
				}
				if record.Type == route53.RRTypeCname {
					got = append(got, record)
				}
			}

			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("ingress records mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestReconcileFailsWhileTheIngressServiceHasNoAddress(t *testing.T) {
	f := setup(t)
	createService(t, newIngressService("ingress-controller", "ingress-nginx", ""))

	err := f.service.ReconcileRoute53(context.Background())
	if !IsIngressNotReady(err) {
		t.Fatalf("error = %v, want an ingress not ready error", err)
	}
}

func TestReconcileFailsWithTwoIngressServices(t *testing.T) {
	f := setup(t)
	createService(t, newIngressService("ingress-controller", "ingress-nginx", "10.0.0.2"))
	createService(t, newIngressService("other-ingress-controller", "nginx-ingress-controller", "10.0.0.3"))

	err := f.service.ReconcileRoute53(context.Background())
	if !IsTooManyICServices(err) {
		t.Fatalf("error = %v, want a too many ingress controller services error", err)
	}
}

func TestReconcileIgnoresANonLoadBalancerIngressService(t *testing.T) {
	f := setup(t)

	service := newIngressService("ingress-controller", "ingress-nginx", "")
	service.Spec.Type = corev1.ServiceTypeClusterIP
	createService(t, service)

	if err := f.service.ReconcileRoute53(context.Background()); err != nil {
		t.Fatalf("ReconcileRoute53: %v", err)
	}

	if _, ok := f.route53.Record(f.clusterZoneID(t), "ingress."+clusterDomain(), route53.RRTypeA); ok {
		t.Errorf("a cluster IP service must not produce an ingress record")
	}
}

func TestReconcileUsesTheIngressHostnameAnnotation(t *testing.T) {
	f := setup(t)

	service := newIngressService("ingress-controller", "ingress-nginx", "10.0.0.2")
	service.Annotations = map[string]string{
		"external-dns.alpha.kubernetes.io/hostname": "custom." + clusterDomain(),
	}
	createService(t, service)

	if err := f.service.ReconcileRoute53(context.Background()); err != nil {
		t.Fatalf("ReconcileRoute53: %v", err)
	}

	zoneID := f.clusterZoneID(t)
	if _, ok := f.route53.Record(zoneID, "custom."+clusterDomain(), route53.RRTypeA); !ok {
		t.Errorf("the annotated hostname has no A record")
	}
	if _, ok := f.route53.Record(zoneID, "ingress."+clusterDomain(), route53.RRTypeA); ok {
		t.Errorf("the default ingress hostname must not get a record")
	}
}

func TestReconcileUsesTheWildcardCNAMETarget(t *testing.T) {
	f := setup(t)
	f.scope.wildcardCNAMETarget = "custom"
	createService(t, newIngressService("ingress-controller", "ingress-nginx", "10.0.0.2"))

	if err := f.service.ReconcileRoute53(context.Background()); err != nil {
		t.Fatalf("ReconcileRoute53: %v", err)
	}

	record, ok := f.route53.Record(f.clusterZoneID(t), "*."+clusterDomain(), route53.RRTypeCname)
	if !ok {
		t.Fatalf("there is no wildcard record")
	}
	if diff := cmp.Diff([]string{"custom." + clusterDomain()}, record.Values); diff != "" {
		t.Errorf("wildcard target mismatch (-want +got):\n%s", diff)
	}
}

func TestReconcileWritesTheIngressRecordsOnlyOnce(t *testing.T) {
	f := setup(t)
	createService(t, newIngressService("ingress-controller", "ingress-nginx", "10.0.0.2"))

	if err := f.service.ReconcileRoute53(context.Background()); err != nil {
		t.Fatalf("first ReconcileRoute53: %v", err)
	}

	f.route53.ResetCalls()

	if err := f.service.ReconcileRoute53(context.Background()); err != nil {
		t.Fatalf("second ReconcileRoute53: %v", err)
	}

	if got := f.route53.CallsTo(fakeroute53.OpChangeResourceRecordSet); got != 0 {
		t.Errorf("ChangeResourceRecordSets calls = %d, want 0, because nothing changed", got)
	}
}

func TestReconcileWithoutGatewayServicesWritesNoGatewayRecords(t *testing.T) {
	f := setup(t)

	if err := f.service.ReconcileRoute53(context.Background()); err != nil {
		t.Fatalf("ReconcileRoute53: %v", err)
	}

	if _, ok := f.route53.Record(f.clusterZoneID(t), "gateway."+clusterDomain(), route53.RRTypeA); ok {
		t.Errorf("a gateway record was written without a gateway service")
	}
}

func TestReconcileCreatesGatewayRecords(t *testing.T) {
	f := setup(t)
	createService(t, newGatewayService("gateway-one", "one."+clusterDomain(), "10.0.1.1"))
	createService(t, newGatewayService("gateway-two", "two."+clusterDomain(), "10.0.1.2"))

	if err := f.service.ReconcileRoute53(context.Background()); err != nil {
		t.Fatalf("ReconcileRoute53: %v", err)
	}

	zoneID := f.clusterZoneID(t)

	for host, ip := range map[string]string{"one": "10.0.1.1", "two": "10.0.1.2"} {
		record, ok := f.route53.Record(zoneID, host+"."+clusterDomain(), route53.RRTypeA)
		if !ok {
			t.Fatalf("gateway %s has no record", host)
		}
		if diff := cmp.Diff([]string{ip}, record.Values); diff != "" {
			t.Errorf("gateway %s value mismatch (-want +got):\n%s", host, diff)
		}
	}
}

func TestReconcileSkipsGatewayServicesWhichDoNotQualify(t *testing.T) {
	unmanaged := newGatewayService("unmanaged", "unmanaged."+clusterDomain(), "10.0.1.3")
	delete(unmanaged.Annotations, "giantswarm.io/external-dns")

	noHostname := newGatewayService("no-hostname", "", "10.0.1.4")

	// A cluster IP service cannot carry a load balancer address at all, so this
	// case only shows that the operator skips a service of the wrong type.
	clusterIP := newGatewayService("cluster-ip", "clusterip."+clusterDomain(), "")
	clusterIP.Spec.Type = corev1.ServiceTypeClusterIP

	noAddress := newGatewayService("no-address", "noaddress."+clusterDomain(), "")

	for _, tc := range []struct {
		name     string
		service  *corev1.Service
		hostname string
	}{
		{"without the managed annotation", unmanaged, "unmanaged." + clusterDomain()},
		{"without a hostname", noHostname, ""},
		{"without a load balancer", clusterIP, "clusterip." + clusterDomain()},
		{"without an address", noAddress, "noaddress." + clusterDomain()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t)
			createService(t, tc.service)

			if err := f.service.ReconcileRoute53(context.Background()); err != nil {
				t.Fatalf("ReconcileRoute53: %v", err)
			}

			if tc.hostname == "" {
				return
			}
			if _, ok := f.route53.Record(f.clusterZoneID(t), tc.hostname, route53.RRTypeA); ok {
				t.Errorf("%s produced a record", tc.name)
			}
		})
	}
}

// The operator translates a few AWS error codes into its own error kinds, so
// that the controller can tell them apart.
func TestReconcileTranslatesAWSErrors(t *testing.T) {
	for _, tc := range []struct {
		name  string
		err   error
		check func(error) bool
	}{
		{
			name:  "hosted zone not found",
			err:   awserr.New(route53.ErrCodeHostedZoneNotFound, "hosted zone not found", nil),
			check: IsHostedZoneNotFound,
		},
		{
			name:  "throttling",
			err:   awserr.New(route53.ErrCodeThrottlingException, "slow down", nil),
			check: IsThrottlingRateExceededError,
		},
		{
			name:  "invalid change batch for a missing record",
			err:   awserr.New(route53.ErrCodeInvalidChangeBatch, "the record was not found", nil),
			check: IsNotFound,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t)
			f.route53.FailNext(fakeroute53.OpChangeResourceRecordSet, tc.err)

			err := f.service.ReconcileRoute53(context.Background())
			if !tc.check(err) {
				t.Fatalf("error = %v, want the translated form of %v", err, tc.err)
			}
		})
	}
}

// clusterZoneID returns the ID of the hosted zone of the test cluster. The base
// hosted zone holds an NS delegation with the same name, so it is excluded.
func (f *fixture) clusterZoneID(t *testing.T) string {
	t.Helper()

	for _, id := range f.route53.ZoneIDs() {
		if id == f.baseZoneID {
			continue
		}
		if _, ok := f.route53.Record(id, clusterDomain(), route53.RRTypeNs); ok {
			return id
		}
	}

	t.Fatalf("there is no hosted zone for %s", clusterDomain())
	return ""
}
