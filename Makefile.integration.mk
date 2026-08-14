##@ Integration tests

# Local only. These targets are deliberately not wired into CI.

ENVTEST_K8S_VERSION ?= 1.34.0
ENVTEST_VERSION     ?= release-0.24
# Keep CAPI_VERSION in sync with sigs.k8s.io/cluster-api in go.mod.
CAPI_VERSION        ?= v1.13.4

LOCALBIN            := $(shell pwd)/bin
SETUP_ENVTEST       := $(LOCALBIN)/setup-envtest
CAPI_CLUSTER_CRD    := test/crds/cluster.x-k8s.io_clusters.yaml
CAPI_CRD_URL        := https://raw.githubusercontent.com/kubernetes-sigs/cluster-api/$(CAPI_VERSION)/config/crd/bases/cluster.x-k8s.io_clusters.yaml

.PHONY: test-integration
test-integration: $(SETUP_ENVTEST) $(CAPI_CLUSTER_CRD) ## Runs the local integration tests (not run in CI).
	@echo "====> $@"
	KUBEBUILDER_ASSETS="$$($(SETUP_ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" \
		go test -tags=integration -count=1 -p=1 -timeout 15m ./...

.PHONY: test-integration-tools
test-integration-tools: $(SETUP_ENVTEST) $(CAPI_CLUSTER_CRD) ## Downloads the integration test tools and fixtures.

$(SETUP_ENVTEST):
	@echo "====> setup-envtest"
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@$(ENVTEST_VERSION)

$(CAPI_CLUSTER_CRD):
	@echo "====> $(CAPI_CLUSTER_CRD)"
	mkdir -p $(dir $(CAPI_CLUSTER_CRD))
	curl -sSfLo $(CAPI_CLUSTER_CRD) $(CAPI_CRD_URL)
