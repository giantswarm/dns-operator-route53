package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/allegro/bigcache/v3"
)

var DNSOperatorCache *bigcache.BigCache

const (
	clusterIngressRecordsPrefix = "ingressRecords"
	zoneRecordsPrefix           = "zoneRecords"
	zoneIDPrefix                = "zoneID"
	nameserverRecordsPrefix     = "nameserverRecords"
	clusterGatewayRecordsPrefix = "gatewayRecords"

	ClusterIngressRecords = 1
	ZoneRecords           = 2
	ZoneID                = 3
	NameserverRecords     = 4
	ClusterGatewayRecords = 5

	unknownCacheIDError = "unknown cache identifier"
)

// Setting an own cache config as the default configuration will lead in
// +80 MB in working_set_bytes
var config = bigcache.Config{
	// number of shards (must be a power of 2)
	Shards: 256,

	// time after which entry can be evicted
	LifeWindow: 6 * time.Minute,

	// Interval between removing expired entries (clean up).
	// If set to <= 0 then no action is performed.
	// Setting to < 1 second is counterproductive — bigcache has a one second resolution.
	CleanWindow: 5 * time.Minute,

	// rps * lifeWindow, used only in initial memory allocation
	MaxEntriesInWindow: 1000 * 10 * 60,

	// max entry size in bytes, used only in initial memory allocation
	MaxEntrySize: 500,

	// prints information about additional memory allocation
	Verbose: true,

	// cache will not allocate more memory than this limit, value in MB
	// if value is reached then the oldest entries can be overridden for the new ones
	// 0 value means no size limit
	HardMaxCacheSize: 24,

	// callback fired when the oldest entry is removed because of its expiration time or no space left
	// for the new entry, or because delete was called. A bitmask representing the reason will be returned.
	// Default value is nil which means no callback and it prevents from unwrapping the oldest entry.
	OnRemove: nil,

	// OnRemoveWithReason is a callback fired when the oldest entry is removed because of its expiration time or no space left
	// for the new entry, or because delete was called. A constant representing the reason will be passed through.
	// Default value is nil which means no callback and it prevents from unwrapping the oldest entry.
	// Ignored if OnRemove is specified.
	OnRemoveWithReason: nil,
}

func NewDNSOperatorCache() (*bigcache.BigCache, error) {
	return bigcache.New(context.Background(), config)
}

func GetDNSCacheRecord(recordID int, keySuffix string) ([]byte, error) {

	switch recordID {
	case ClusterIngressRecords:
		return DNSOperatorCache.Get(fmt.Sprintf("%s-%s", clusterIngressRecordsPrefix, keySuffix))
	case ZoneRecords:
		return DNSOperatorCache.Get(fmt.Sprintf("%s-%s", zoneRecordsPrefix, keySuffix))
	case ZoneID:
		return DNSOperatorCache.Get(fmt.Sprintf("%s-%s", zoneIDPrefix, keySuffix))
	case NameserverRecords:
		return DNSOperatorCache.Get(fmt.Sprintf("%s-%s", nameserverRecordsPrefix, keySuffix))
	case ClusterGatewayRecords:
		return DNSOperatorCache.Get(fmt.Sprintf("%s-%s", clusterGatewayRecordsPrefix, keySuffix))
	default:
		return nil, errors.New(unknownCacheIDError)
	}

}

func SetDNSCacheRecord(recordID int, keySuffix string, data []byte) error {

	switch recordID {
	case ClusterIngressRecords:
		return DNSOperatorCache.Set(fmt.Sprintf("%s-%s", clusterIngressRecordsPrefix, keySuffix), data)
	case ZoneRecords:
		return DNSOperatorCache.Set(fmt.Sprintf("%s-%s", zoneRecordsPrefix, keySuffix), data)
	case ZoneID:
		return DNSOperatorCache.Set(fmt.Sprintf("%s-%s", zoneIDPrefix, keySuffix), data)
	case NameserverRecords:
		return DNSOperatorCache.Set(fmt.Sprintf("%s-%s", nameserverRecordsPrefix, keySuffix), data)
	case ClusterGatewayRecords:
		return DNSOperatorCache.Set(fmt.Sprintf("%s-%s", clusterGatewayRecordsPrefix, keySuffix), data)
	default:
		return errors.New(unknownCacheIDError)
	}

}

func DeleteDNSCacheRecord(recordID int, keySuffix string) error {

	switch recordID {
	case ClusterIngressRecords:
		return DNSOperatorCache.Delete(fmt.Sprintf("%s-%s", clusterIngressRecordsPrefix, keySuffix))
	case ZoneRecords:
		return DNSOperatorCache.Delete(fmt.Sprintf("%s-%s", zoneRecordsPrefix, keySuffix))
	case ZoneID:
		return DNSOperatorCache.Delete(fmt.Sprintf("%s-%s", zoneIDPrefix, keySuffix))
	case NameserverRecords:
		return DNSOperatorCache.Delete(fmt.Sprintf("%s-%s", nameserverRecordsPrefix, keySuffix))
	case ClusterGatewayRecords:
		return DNSOperatorCache.Delete(fmt.Sprintf("%s-%s", clusterGatewayRecordsPrefix, keySuffix))
	default:
		return errors.New(unknownCacheIDError)
	}

}
