package cache

import (
	"errors"
	"testing"

	"github.com/allegro/bigcache/v3"
)

// newTestCache installs a cache for the test and removes it afterwards.
func newTestCache(t *testing.T) {
	t.Helper()

	previous := DNSOperatorCache

	cache, err := NewDNSOperatorCache()
	if err != nil {
		t.Fatalf("failed to create the cache: %v", err)
	}
	DNSOperatorCache = cache

	t.Cleanup(func() { DNSOperatorCache = previous })
}

func TestCacheRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name     string
		recordID int
		prefix   string
	}{
		{"cluster ingress records", ClusterIngressRecords, "ingressRecords"},
		{"zone records", ZoneRecords, "zoneRecords"},
		{"zone ID", ZoneID, "zoneID"},
		{"nameserver records", NameserverRecords, "nameserverRecords"},
		{"cluster gateway records", ClusterGatewayRecords, "gatewayRecords"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			newTestCache(t)

			if err := SetDNSCacheRecord(tc.recordID, "foo", []byte("value")); err != nil {
				t.Fatalf("SetDNSCacheRecord: %v", err)
			}

			got, err := GetDNSCacheRecord(tc.recordID, "foo")
			if err != nil {
				t.Fatalf("GetDNSCacheRecord: %v", err)
			}
			if string(got) != "value" {
				t.Errorf("value = %s, want value", got)
			}

			// Each record kind has its own key prefix, so a different kind with
			// the same suffix must not answer.
			raw, err := DNSOperatorCache.Get(tc.prefix + "-foo")
			if err != nil {
				t.Fatalf("the entry is not stored under %s-foo: %v", tc.prefix, err)
			}
			if string(raw) != "value" {
				t.Errorf("raw value = %s, want value", raw)
			}

			if err := DeleteDNSCacheRecord(tc.recordID, "foo"); err != nil {
				t.Fatalf("DeleteDNSCacheRecord: %v", err)
			}
			if _, err := GetDNSCacheRecord(tc.recordID, "foo"); !errors.Is(err, bigcache.ErrEntryNotFound) {
				t.Errorf("error after the delete = %v, want an entry not found error", err)
			}
		})
	}
}

func TestCacheSeparatesTheRecordKinds(t *testing.T) {
	newTestCache(t)

	if err := SetDNSCacheRecord(ZoneID, "foo", []byte("zone")); err != nil {
		t.Fatalf("SetDNSCacheRecord: %v", err)
	}

	if _, err := GetDNSCacheRecord(ZoneRecords, "foo"); !errors.Is(err, bigcache.ErrEntryNotFound) {
		t.Errorf("error = %v, want an entry not found error for another record kind", err)
	}
}

func TestCacheRejectsAnUnknownRecordID(t *testing.T) {
	newTestCache(t)

	const unknown = 99

	if _, err := GetDNSCacheRecord(unknown, "foo"); err == nil {
		t.Errorf("GetDNSCacheRecord returned no error for an unknown record ID")
	}
	if err := SetDNSCacheRecord(unknown, "foo", []byte("value")); err == nil {
		t.Errorf("SetDNSCacheRecord returned no error for an unknown record ID")
	}
	if err := DeleteDNSCacheRecord(unknown, "foo"); err == nil {
		t.Errorf("DeleteDNSCacheRecord returned no error for an unknown record ID")
	}
}
