package cache

import (
	"time"

	"github.com/allegro/bigcache/v3"
)

var DNSOperatorCache *bigcache.BigCache

const (
	ClusterIngressRecordsPrefix = "ingressRecords"
	ZoneRecordsPrefix           = "zoneRecords"
	ZoneIDPrefix                = "zoneID"
	NameserverRecordsPrefix     = "nameserverRecords"
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

// NewDNSOperatorCache create a new BigCache with our custom configuration
func NewDNSOperatorCache() (*bigcache.BigCache, error) {
	return bigcache.NewBigCache(config)
}
