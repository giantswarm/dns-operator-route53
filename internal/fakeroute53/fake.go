// Package fakeroute53 holds an in-memory implementation of the Route53 API.
//
// The fake keeps hosted zones and their record sets in memory and applies change
// batches to them. Tests can therefore make assertions about the resulting DNS
// state instead of the sequence of API calls. The fake also counts the calls, so
// a test can show that a cache hit makes no API call at all.
package fakeroute53

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/aws/aws-sdk-go/service/route53/route53iface"
)

// Operation names. Use them with FailNext and to read the call log.
const (
	OpListHostedZonesByName   = "ListHostedZonesByName"
	OpCreateHostedZone        = "CreateHostedZone"
	OpDeleteHostedZone        = "DeleteHostedZone"
	OpListResourceRecordSets  = "ListResourceRecordSets"
	OpChangeResourceRecordSet = "ChangeResourceRecordSets"
)

// Record is a comparable view of a resource record set.
type Record struct {
	Name   string
	Type   string
	TTL    int64
	Values []string
}

// Fake implements the part of route53iface.Route53API which the operator uses.
// The embedded interface satisfies the remaining methods. A call to one of them
// panics, which shows an unexpected API call.
type Fake struct {
	route53iface.Route53API

	mu     sync.Mutex
	zones  map[string]*zone
	calls  []string
	errs   map[string]error
	nextID int
}

type zone struct {
	id      string
	name    string
	records []*route53.ResourceRecordSet
}

// New returns an empty fake.
func New() *Fake {
	return &Fake{
		zones: map[string]*zone{},
		errs:  map[string]error{},
	}
}

// AddZone adds a hosted zone with an apex NS record and an apex SOA record. The
// name gets a trailing dot if it has none. The returned ID is the full ID, as
// the Route53 API reports it.
func (f *Fake) AddZone(name string) string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.addZone(name)
}

func (f *Fake) addZone(name string) string {
	f.nextID++
	id := fmt.Sprintf("/hostedzone/Z%08d", f.nextID)
	name = normalizeName(name)

	f.zones[zoneKey(id)] = &zone{
		id:   id,
		name: name,
		records: []*route53.ResourceRecordSet{
			// The operator reads the first record set of a zone and expects the
			// NS record, so the order matters.
			newRecordSet(name, route53.RRTypeNs, 172800,
				"ns-1.awsdns-01.com.", "ns-2.awsdns-02.net."),
			newRecordSet(name, route53.RRTypeSoa, 900,
				"ns-1.awsdns-01.com. awsdns-hostmaster.amazon.com. 1 7200 900 1209600 86400"),
		},
	}

	return id
}

// AddRecord adds a record set to a hosted zone, or replaces the record set which
// has the same name and type.
func (f *Fake) AddRecord(zoneID, name, recordType string, ttl int64, values ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	z, ok := f.zones[zoneKey(zoneID)]
	if !ok {
		return
	}

	z.upsert(newRecordSet(name, recordType, ttl, values...))
}

// FailNext makes the next call to the given operation return err.
func (f *Fake) FailNext(operation string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.errs[operation] = err
}

// Calls returns the operation names of every call so far, in order.
func (f *Fake) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]string(nil), f.calls...)
}

// CallsTo returns how many times the given operation was called.
func (f *Fake) CallsTo(operation string) int {
	f.mu.Lock()
	defer f.mu.Unlock()

	var count int
	for _, call := range f.calls {
		if call == operation {
			count++
		}
	}
	return count
}

// ResetCalls empties the call log.
func (f *Fake) ResetCalls() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls = nil
}

// ZoneIDs returns the IDs of all hosted zones, sorted.
func (f *Fake) ZoneIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	ids := make([]string, 0, len(f.zones))
	for _, z := range f.zones {
		ids = append(ids, z.id)
	}
	sort.Strings(ids)
	return ids
}

// HasZone reports whether a hosted zone with the given ID exists.
func (f *Fake) HasZone(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	_, ok := f.zones[zoneKey(id)]
	return ok
}

// Records returns the record sets of a hosted zone, sorted by name and type.
func (f *Fake) Records(id string) []Record {
	f.mu.Lock()
	defer f.mu.Unlock()

	z, ok := f.zones[zoneKey(id)]
	if !ok {
		return nil
	}

	records := make([]Record, 0, len(z.records))
	for _, set := range z.records {
		record := Record{
			Name: aws.StringValue(set.Name),
			Type: aws.StringValue(set.Type),
			TTL:  aws.Int64Value(set.TTL),
		}
		for _, value := range set.ResourceRecords {
			record.Values = append(record.Values, aws.StringValue(value.Value))
		}
		records = append(records, record)
	}

	sort.Slice(records, func(i, j int) bool {
		if records[i].Name != records[j].Name {
			return records[i].Name < records[j].Name
		}
		return records[i].Type < records[j].Type
	})

	return records
}

// Record returns a single record set by name and type. The name gets a trailing
// dot if it has none.
func (f *Fake) Record(id, name, recordType string) (Record, bool) {
	for _, record := range f.Records(id) {
		if record.Name == normalizeName(name) && record.Type == recordType {
			return record, true
		}
	}
	return Record{}, false
}

func (f *Fake) ListHostedZonesByNameWithContext(_ aws.Context, input *route53.ListHostedZonesByNameInput, _ ...request.Option) (*route53.ListHostedZonesByNameOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if err := f.record(OpListHostedZonesByName); err != nil {
		return nil, err
	}

	// The real API returns the zones in name order, starting at the requested
	// name. The operator only reads the first entry and compares its name.
	zones := make([]*zone, 0, len(f.zones))
	for _, z := range f.zones {
		zones = append(zones, z)
	}
	sort.Slice(zones, func(i, j int) bool { return zones[i].name < zones[j].name })

	from := normalizeName(aws.StringValue(input.DNSName))

	out := &route53.ListHostedZonesByNameOutput{}
	for _, z := range zones {
		if z.name < from {
			continue
		}
		out.HostedZones = append(out.HostedZones, &route53.HostedZone{
			Id:   aws.String(z.id),
			Name: aws.String(z.name),
		})
	}

	return out, nil
}

func (f *Fake) CreateHostedZoneWithContext(_ aws.Context, input *route53.CreateHostedZoneInput, _ ...request.Option) (*route53.CreateHostedZoneOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if err := f.record(OpCreateHostedZone); err != nil {
		return nil, err
	}

	id := f.addZone(aws.StringValue(input.Name))

	return &route53.CreateHostedZoneOutput{
		HostedZone: &route53.HostedZone{
			Id:     aws.String(id),
			Name:   aws.String(normalizeName(aws.StringValue(input.Name))),
			Config: input.HostedZoneConfig,
		},
	}, nil
}

func (f *Fake) DeleteHostedZoneWithContext(_ aws.Context, input *route53.DeleteHostedZoneInput, _ ...request.Option) (*route53.DeleteHostedZoneOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if err := f.record(OpDeleteHostedZone); err != nil {
		return nil, err
	}

	key := zoneKey(aws.StringValue(input.Id))
	if _, ok := f.zones[key]; !ok {
		return nil, noSuchHostedZone(aws.StringValue(input.Id))
	}
	delete(f.zones, key)

	return &route53.DeleteHostedZoneOutput{}, nil
}

func (f *Fake) ListResourceRecordSets(input *route53.ListResourceRecordSetsInput) (*route53.ListResourceRecordSetsOutput, error) {
	return f.ListResourceRecordSetsWithContext(nil, input)
}

func (f *Fake) ListResourceRecordSetsWithContext(_ aws.Context, input *route53.ListResourceRecordSetsInput, _ ...request.Option) (*route53.ListResourceRecordSetsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if err := f.record(OpListResourceRecordSets); err != nil {
		return nil, err
	}

	z, ok := f.zones[zoneKey(aws.StringValue(input.HostedZoneId))]
	if !ok {
		return nil, noSuchHostedZone(aws.StringValue(input.HostedZoneId))
	}

	sets := append([]*route53.ResourceRecordSet(nil), z.records...)
	if input.MaxItems != nil {
		max, err := strconv.Atoi(aws.StringValue(input.MaxItems))
		if err == nil && max < len(sets) {
			sets = sets[:max]
		}
	}

	return &route53.ListResourceRecordSetsOutput{
		ResourceRecordSets: sets,
		IsTruncated:        aws.Bool(len(sets) < len(z.records)),
	}, nil
}

func (f *Fake) ChangeResourceRecordSetsWithContext(_ aws.Context, input *route53.ChangeResourceRecordSetsInput, _ ...request.Option) (*route53.ChangeResourceRecordSetsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if err := f.record(OpChangeResourceRecordSet); err != nil {
		return nil, err
	}

	z, ok := f.zones[zoneKey(aws.StringValue(input.HostedZoneId))]
	if !ok {
		return nil, noSuchHostedZone(aws.StringValue(input.HostedZoneId))
	}

	for _, change := range input.ChangeBatch.Changes {
		set := *change.ResourceRecordSet
		set.Name = aws.String(normalizeName(aws.StringValue(set.Name)))

		switch aws.StringValue(change.Action) {
		case route53.ChangeActionUpsert:
			z.upsert(&set)
		case route53.ChangeActionDelete:
			if !z.delete(&set) {
				return nil, awserr.New(route53.ErrCodeInvalidChangeBatch,
					fmt.Sprintf("Tried to delete resource record set [name='%s', type='%s'] but it was not found",
						aws.StringValue(set.Name), aws.StringValue(set.Type)), nil)
			}
		default:
			return nil, awserr.New(route53.ErrCodeInvalidChangeBatch,
				fmt.Sprintf("unsupported action %q", aws.StringValue(change.Action)), nil)
		}
	}

	return &route53.ChangeResourceRecordSetsOutput{}, nil
}

// record appends the operation to the call log and returns an injected error.
// The caller must hold the lock.
func (f *Fake) record(operation string) error {
	f.calls = append(f.calls, operation)

	if err, ok := f.errs[operation]; ok {
		delete(f.errs, operation)
		return err
	}

	return nil
}

func (z *zone) upsert(set *route53.ResourceRecordSet) {
	for i, existing := range z.records {
		if sameRecordSet(existing, set) {
			z.records[i] = set
			return
		}
	}
	z.records = append(z.records, set)
}

func (z *zone) delete(set *route53.ResourceRecordSet) bool {
	for i, existing := range z.records {
		if sameRecordSet(existing, set) {
			z.records = append(z.records[:i], z.records[i+1:]...)
			return true
		}
	}
	return false
}

func sameRecordSet(a, b *route53.ResourceRecordSet) bool {
	return aws.StringValue(a.Name) == aws.StringValue(b.Name) &&
		aws.StringValue(a.Type) == aws.StringValue(b.Type)
}

func newRecordSet(name, recordType string, ttl int64, values ...string) *route53.ResourceRecordSet {
	set := &route53.ResourceRecordSet{
		Name: aws.String(normalizeName(name)),
		Type: aws.String(recordType),
		TTL:  aws.Int64(ttl),
	}
	for _, value := range values {
		set.ResourceRecords = append(set.ResourceRecords, &route53.ResourceRecord{
			Value: aws.String(value),
		})
	}
	return set
}

func noSuchHostedZone(id string) error {
	return awserr.New(route53.ErrCodeNoSuchHostedZone, fmt.Sprintf("no such hosted zone %q", id), nil)
}

// normalizeName adds the trailing dot which Route53 stores.
func normalizeName(name string) string {
	if name == "" || strings.HasSuffix(name, ".") {
		return name
	}
	return name + "."
}

// zoneKey accepts both the bare ID and the full "/hostedzone/<id>" form.
func zoneKey(id string) string {
	return strings.TrimPrefix(id, "/hostedzone/")
}
