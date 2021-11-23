module github.com/giantswarm/dns-operator-openstack

go 1.13

require (
	github.com/aws/aws-sdk-go v1.40.56
	github.com/giantswarm/dns-operator-aws v0.2.0
	github.com/go-logr/logr v1.2.0
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.11.0
	k8s.io/api v0.23.0-alpha.4
	k8s.io/apimachinery v0.23.0-alpha.4
	k8s.io/client-go v0.23.0-alpha.4
	k8s.io/component-base v0.23.0-alpha.4
	k8s.io/klog v1.0.0
	sigs.k8s.io/cluster-api v1.0.1-0.20211111175208-4cc2fce2111a
	sigs.k8s.io/cluster-api-provider-aws v1.1.0
	sigs.k8s.io/cluster-api-provider-openstack v0.5.0
	sigs.k8s.io/controller-runtime v0.11.0-beta.0.0.20211110210527-619e6b92dab9
)

replace (
	github.com/coreos/etcd v3.3.10+incompatible => github.com/coreos/etcd v3.3.25+incompatible
	github.com/dgrijalva/jwt-go => github.com/dgrijalva/jwt-go/v4 v4.0.0-preview1
	github.com/gogo/protobuf v1.3.1 => github.com/gogo/protobuf v1.3.2
	github.com/gorilla/websocket v1.4.0 => github.com/gorilla/websocket v1.4.2
	sigs.k8s.io/cluster-api => sigs.k8s.io/cluster-api v1.0.1-0.20211028151834-d72fd59c8483
)
