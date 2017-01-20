#!/bin/bash

# Fetch linters
go get -u github.com/alecthomas/gometalinter

# Vendorize bazel dependencies
bin/bazel_to_go.py > /dev/null

# Remove doubly-vendorized k8s dependencies
rm -rf vendor/k8s.io/client-go/vendor
