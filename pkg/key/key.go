package key

const (
	DNSFinalizerNameOld = "dns-operator-openstack.finalizers.giantswarm.io"
	DNSFinalizerNameNew = "dns-operator-route53.finalizers.giantswarm.io"

	AnnotationWildcardCNAMETarget = "dns-operator-route53.giantswarm.io/wildcard-cname-target"
)
