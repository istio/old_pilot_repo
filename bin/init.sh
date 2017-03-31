#!/bin/bash
set -ex

# Vendorize bazel dependencies
bin/bazel_to_go.py > /dev/null

# Remove doubly-vendorized k8s dependencies
rm -rf vendor/k8s.io/client-go/vendor

# Link gen files
mkdir -p vendor/istio.io/api/proxy/v1/config
ln -sf "$(pwd)/bazel-genfiles/external/io_istio_api/proxy/v1/config/cfg.pb.go" \
  vendor/istio.io/api/proxy/v1/config/cfg.pb.go

# Some linters expect the code to be installed
go install ./...
