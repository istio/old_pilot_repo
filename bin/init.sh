#!/bin/bash
set -ex

# Vendorize bazel dependencies
bin/bazel_to_go.py > /dev/null

# Remove doubly-vendorized k8s dependencies
rm -rf vendor/k8s.io/client-go/vendor

# Install mockgen tool to generate golang mock interfaces
go get github.com/golang/mock/mockgen

go install ./...
go get -u github.com/alecthomas/gometalinter
go get -u github.com/bazelbuild/buildifier/buildifier
gometalinter --install --vendored-linters
