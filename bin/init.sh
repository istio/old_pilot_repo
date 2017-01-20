#!/bin/bash
set -ex

# Fetch linters
go get -u github.com/alecthomas/gometalinter
gometalinter --install

# Vendorize bazel dependencies
bin/bazel_to_go.py > /dev/null

# Remove doubly-vendorized k8s dependencies
rm -rf vendor/k8s.io/client-go/vendor
