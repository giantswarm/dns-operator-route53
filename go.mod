module github.com/giantswarm/dns-operator-route53

go 1.18

require (
	github.com/allegro/bigcache/v3 v3.1.0
	github.com/aws/aws-sdk-go v1.45.0
	github.com/giantswarm/k8sclient/v7 v7.0.1
	github.com/giantswarm/microerror v0.4.0
	github.com/giantswarm/micrologger v1.0.0
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.16.0
	golang.org/x/text v0.12.0
	k8s.io/api v0.23.0
	k8s.io/apimachinery v0.23.0
	k8s.io/client-go v0.23.0
	k8s.io/component-base v0.23.0
	sigs.k8s.io/cluster-api v1.1.3
	sigs.k8s.io/controller-runtime v0.11.1
)

require (
	cloud.google.com/go v0.93.3 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver v3.5.1+incompatible // indirect
	github.com/cenkalti/backoff/v4 v4.1.2 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/evanphx/json-patch v4.12.0+incompatible // indirect
	github.com/fsnotify/fsnotify v1.5.1 // indirect
	github.com/giantswarm/backoff v1.0.0 // indirect
	github.com/go-kit/log v0.2.1 // indirect
	github.com/go-logfmt/logfmt v0.5.1 // indirect
	github.com/go-logr/logr v1.2.2 // indirect
	github.com/go-logr/zapr v1.2.0 // indirect
	github.com/go-stack/stack v1.8.1 // indirect
	github.com/gobuffalo/flect v0.2.4 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/uuid v1.2.0 // indirect
	github.com/googleapis/gnostic v0.5.5 // indirect
	github.com/imdario/mergo v0.3.12 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/onsi/gomega v1.17.0 // indirect
	github.com/prometheus/client_model v0.3.0 // indirect
	github.com/prometheus/common v0.42.0 // indirect
	github.com/prometheus/procfs v0.10.1 // indirect
	github.com/rogpeppe/go-internal v1.10.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	go.uber.org/atomic v1.9.0 // indirect
	go.uber.org/multierr v1.7.0 // indirect
	go.uber.org/zap v1.19.1 // indirect
	golang.org/x/net v0.7.0 // indirect
	golang.org/x/oauth2 v0.5.0 // indirect
	golang.org/x/sys v0.8.0 // indirect
	golang.org/x/term v0.5.0 // indirect
	golang.org/x/time v0.0.0-20220922220347-f3bd1da661af // indirect
	gomodules.xyz/jsonpatch/v2 v2.2.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/protobuf v1.30.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/apiextensions-apiserver v0.23.0 // indirect
	k8s.io/klog/v2 v2.30.0 // indirect
	k8s.io/kube-openapi v0.0.0-20211115234752-e816edb12b65 // indirect
	k8s.io/utils v0.0.0-20210930125809-cb0fa318a74b // indirect
	sigs.k8s.io/json v0.0.0-20211020170558-c049b76a60c6 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.0 // indirect
	sigs.k8s.io/yaml v1.3.0 // indirect
)

replace (
	github.com/containerd/containerd => github.com/containerd/containerd v1.7.5
	github.com/coreos/etcd => github.com/coreos/etcd v3.3.27+incompatible
	github.com/dgrijalva/jwt-go => github.com/dgrijalva/jwt-go/v4 v4.0.0-preview1
	github.com/gogo/protobuf v1.3.1 => github.com/gogo/protobuf v1.3.2
	github.com/gorilla/websocket v1.4.0 => github.com/gorilla/websocket v1.4.2
	sigs.k8s.io/cluster-api => sigs.k8s.io/cluster-api v1.0.1-0.20211028151834-d72fd59c8483
)

// fixes some CVEs
replace (
	github.com/Microsoft/hcsshim v0.8.7 => github.com/Microsoft/hcsshim v0.9.2
	github.com/containerd/imgcrypt v1.1.1 => github.com/containerd/imgcrypt v1.1.5
	github.com/nats-io/jwt/v2 => github.com/nats-io/jwt/v2 v2.5.0
	github.com/nats-io/nats-server/v2 v2.1.2 => github.com/nats-io/nats-server/v2 v2.9.11
	github.com/nats-io/nats.go => github.com/nats-io/nats.go v1.28.0
	github.com/opencontainers/image-spec v1.0.1 => github.com/opencontainers/image-spec v1.0.2
	github.com/opencontainers/runc v1.0.2 => github.com/opencontainers/runc v1.1.2
	github.com/pkg/sftp v1.10.1 => github.com/pkg/sftp v1.13.4
	golang.org/x/net v0.5.0 => golang.org/x/net v0.7.0
	golang.org/x/text v0.3.6 => golang.org/x/text v0.3.7
)
