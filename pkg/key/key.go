package key

import (
	capi "sigs.k8s.io/cluster-api/api/v1alpha4"
)

const (
	ClusterNameLabel                        = "cluster.x-k8s.io/cluster-name"
	CAPIWatchFilterLabel                    = "cluster.x-k8s.io/watch-filter"
	CAPAReleaseComponent                    = "cluster-api-provider-aws"
	DNSFinalizerName                        = "dns-operator-openstack.finalizers.giantswarm.io"
	DNSZoneReady         capi.ConditionType = "DNSZoneReady"
)
