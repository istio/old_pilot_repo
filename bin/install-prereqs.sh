#!/bin/bash

set -ex

# Install linters
go get -u gopkg.in/alecthomas/gometalinter.v1
gometalinter --install --update --vendored-linters

# Install buildifier BUILD file validator
go get -u github.com/bazelbuild/buildifier/buildifier
