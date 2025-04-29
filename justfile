set shell := ["bash", "-euo", "pipefail", "-c"]

import "hack/tools.just"

# Print list of available recipes
default:
  @just --list

export CGO_ENABLED := "0"

_gotools:
  go fmt ./...
  go vet {{go_flags}} ./...

# Called in CI
_lint: _license_headers _gotools

# Generate, lint, test and build everything
all: gen lint lint-gha test build kube-build && version

# Run linters against code (incl. license headers)
lint: _lint _golangci_lint
  {{golangci_lint}} run --show-stats ./...

# Run golangci-lint to attempt to fix issues
lint-fix: _lint _golangci_lint
  {{golangci_lint}} run --show-stats --fix ./...

go_flags := "-ldflags=\"-w -s -X go.githedgehog.com/gateway/pkg/version.Version=" + version + "\""
go_build := "go build " + go_flags
go_linux_build := "GOOS=linux GOARCH=amd64 " + go_build

_kube_gen:
  # Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject implementations
  {{controller_gen}} object:headerFile="hack/boilerplate.go.txt" paths="./..."
  # Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects
  {{controller_gen}} rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

# Generate docs, code/manifests, things to embed, etc
gen: _kube_gen _crd_ref_docs
  {{crd_ref_docs}} --source-path=./api/ --config=api/docs.config.yaml --renderer=markdown --output-path=./docs/api.md

# Build all artifacts
build: _license_headers _gotools gen && version
  {{go_linux_build}} -o ./bin/gateway ./cmd
  {{go_linux_build}} -o ./bin/gateway-agent ./cmd/gateway-agent
  # Build complete

oci_repo := "127.0.0.1:30000"
oci_prefix := "githedgehog/gateway"

_helm-gateway-api: _kustomize _helm _kube_gen
  @rm config/helm/gateway-api-v*.tgz || true
  {{kustomize}} build config/crd > config/helm/gateway-api/templates/crds.yaml
  {{helm}} package config/helm/gateway-api --destination config/helm --version {{version}}
  {{helm}} lint config/helm/gateway-api-{{version}}.tgz

_helm-gateway: _kustomize _helm _helmify _kube_gen
  @rm config/helm/gateway-v*.tgz || true
  @rm config/helm/gateway/templates/*.yaml config/helm/gateway/values.yaml || true
  {{kustomize}} build config/default | {{helmify}} config/helm/gateway
  {{helm}} package config/helm/gateway --destination config/helm --version {{version}}
  {{helm}} lint config/helm/gateway-{{version}}.tgz

# Build all K8s artifacts (images and charts)
kube-build: build (_docker-build "gateway") (_docker-build "gateway-agent") _helm-gateway-api _helm-gateway && version
  # Docker images and Helm charts built

# Push all K8s artifacts (images and charts)
kube-push: kube-build (_helm-push "gateway-api") (_kube-push "gateway") (_docker-push "gateway-agent") && version
  # Docker images and Helm charts pushed

# Push all K8s artifacts (images and charts) and binaries
push: kube-push && version

# Install API on a kind cluster and wait for CRDs to be ready
test-api: _helm-gateway-api
    kind export kubeconfig --name kind || kind create cluster --name kind
    kind export kubeconfig --name kind
    {{helm}} install -n default gateway-api config/helm/gateway-api-{{version}}.tgz
    sleep 10
    kubectl wait --for condition=established --timeout=60s crd/peerings.gateway.githedgehog.com
    kubectl get crd | grep gateway
    kind delete cluster --name kind

# Patch deployment using the default kubeconfig (KUBECONFIG env or ~/.kube/config)
patch: && version
  kubectl -n fab patch fab/default --type=merge -p '{"spec":{"overrides":{"versions":{"gateway":{"api":"{{version}}","controller":"{{version}}"}}}}}'
