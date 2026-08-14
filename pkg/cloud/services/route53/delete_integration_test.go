//go:build integration

package route53

import (
	"context"
	"errors"
	"testing"

	"github.com/allegro/bigcache/v3"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/route53"

	"github.com/giantswarm/dns-operator-route53/internal/fakeroute53"
	dnscache "github.com/giantswarm/dns-operator-route53/pkg/cloud/cache"
)

func TestDeleteWithoutAHostedZoneDoesNothing(t *testing.T) {
	f := setup(t)

	if err := f.service.DeleteRoute53(context.Background()); err != nil {
		t.Fatalf("DeleteRoute53: %v", err)
	}

	if got := f.route53.CallsTo(fakeroute53.OpDeleteHostedZone); got != 0 {
		t.Errorf("DeleteHostedZone calls = %d, want 0", got)
	}
	if !f.route53.HasZone(f.baseZoneID) {
		t.Errorf("the base hosted zone was deleted")
	}
}

func TestDeleteRemovesTheRecordsTheDelegationAndTheZone(t *testing.T) {
	f := setup(t)
	createService(t, newIngressService("ingress-controller", "ingress-nginx", "10.0.0.2"))

	if err := f.service.ReconcileRoute53(context.Background()); err != nil {
		t.Fatalf("ReconcileRoute53: %v", err)
	}

	zoneID := f.clusterZoneID(t)

	if err := f.service.DeleteRoute53(context.Background()); err != nil {
		t.Fatalf("DeleteRoute53: %v", err)
	}

	if f.route53.HasZone(zoneID) {
		t.Errorf("the cluster hosted zone still exists")
	}
	if _, ok := f.route53.Record(f.baseZoneID, clusterDomain(), route53.RRTypeNs); ok {
		t.Errorf("the NS delegation is still in the base hosted zone")
	}
}

// The apex records of a zone go away with the zone itself, so the operator must
// not try to delete them.
func TestDeleteKeepsTheApexRecords(t *testing.T) {
	f := setup(t)
	zoneID := f.route53.AddZone(clusterDomain())
	f.route53.AddRecord(zoneID, "api."+clusterDomain(), route53.RRTypeA, 300, "10.0.0.1")

	if err := f.service.DeleteRoute53(context.Background()); err != nil {
		t.Fatalf("DeleteRoute53: %v", err)
	}

	if f.route53.HasZone(zoneID) {
		t.Errorf("the cluster hosted zone still exists")
	}
}

func TestDeleteWithoutClusterRecordsStillDeletesTheZone(t *testing.T) {
	f := setup(t)
	zoneID := f.route53.AddZone(clusterDomain())

	if err := f.service.DeleteRoute53(context.Background()); err != nil {
		t.Fatalf("DeleteRoute53: %v", err)
	}

	if f.route53.HasZone(zoneID) {
		t.Errorf("the cluster hosted zone still exists")
	}
}

// The delegation may already be gone. Route53 answers that with an invalid change
// batch error, which the operator tolerates.
func TestDeleteToleratesAMissingDelegation(t *testing.T) {
	f := setup(t)
	zoneID := f.route53.AddZone(clusterDomain())

	if _, ok := f.route53.Record(f.baseZoneID, clusterDomain(), route53.RRTypeNs); ok {
		t.Fatalf("the base hosted zone must not hold a delegation for this test")
	}

	if err := f.service.DeleteRoute53(context.Background()); err != nil {
		t.Fatalf("DeleteRoute53: %v", err)
	}

	if f.route53.HasZone(zoneID) {
		t.Errorf("the cluster hosted zone still exists")
	}
}

func TestDeleteStopsOnAnotherDelegationError(t *testing.T) {
	f := setup(t)
	zoneID := f.route53.AddZone(clusterDomain())

	// The zone holds apex records only, so the first change is the delegation.
	f.route53.FailNext(fakeroute53.OpChangeResourceRecordSet,
		awserr.New(route53.ErrCodeThrottlingException, "slow down", nil))

	err := f.service.DeleteRoute53(context.Background())
	if !IsThrottlingRateExceededError(err) {
		t.Fatalf("error = %v, want a throttling error", err)
	}

	if !f.route53.HasZone(zoneID) {
		t.Errorf("the cluster hosted zone was deleted despite the error")
	}
}

func TestDeleteReportsAFailureToDeleteTheZone(t *testing.T) {
	f := setup(t)
	f.route53.AddZone(clusterDomain())

	f.route53.FailNext(fakeroute53.OpDeleteHostedZone,
		awserr.New(route53.ErrCodeHostedZoneNotEmpty, "the zone is not empty", nil))

	err := f.service.DeleteRoute53(context.Background())
	if err == nil {
		t.Fatalf("DeleteRoute53 returned no error")
	}
}

func TestDeleteDropsTheCachedZone(t *testing.T) {
	f := setup(t)
	zoneID := f.route53.AddZone(clusterDomain())
	f.route53.AddRecord(zoneID, "api."+clusterDomain(), route53.RRTypeA, 300, "10.0.0.1")

	if err := f.service.ReconcileRoute53(context.Background()); err != nil {
		t.Fatalf("ReconcileRoute53: %v", err)
	}
	if _, err := dnscache.GetDNSCacheRecord(dnscache.ZoneID, clusterName); err != nil {
		t.Fatalf("the zone ID was not cached: %v", err)
	}

	if err := f.service.DeleteRoute53(context.Background()); err != nil {
		t.Fatalf("DeleteRoute53: %v", err)
	}

	if _, err := dnscache.GetDNSCacheRecord(dnscache.ZoneID, clusterName); !errors.Is(err, bigcache.ErrEntryNotFound) {
		t.Errorf("the cached zone ID error = %v, want an entry not found error", err)
	}
	if _, err := dnscache.GetDNSCacheRecord(dnscache.ZoneRecords, zoneID); !errors.Is(err, bigcache.ErrEntryNotFound) {
		t.Errorf("the cached zone records error = %v, want an entry not found error", err)
	}
}
