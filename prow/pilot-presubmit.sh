#!/bin/bash

set -ex

# Test harness will checkout code to directory $GOPATH/src/github.com/istio/pilot
# but we depend on being at path $GOPATH/src/istio.io/pilot for imports
if [ "$CI" == "bootstrap" ]; then
    mkdir -p $GOPATH/src/istio.io
    mv $GOPATH/src/github.com/nlandolfi/pilot $GOPATH/src/istio.io
    cd $GOPATH/src/istio.io/pilot/
fi

# Get configuration for the separate test cluster, it must be at
# ~/.kube and platform/kube because different aspects of testing
# & building require it in each place.
# (c.f., https://github.com/istio/pilot/issues/893, which tracks
# this discrepancy)
gcloud config set container/use_client_certificate True
gcloud container clusters get-credentials testing --zone us-central1-a --project isito-prow
if [ -e platform/kube/config ]; then
    rm platform/kube/config
fi
ln -s ~/.kube/config platform/kube/

echo "=== Bazel Build ==="
./bin/install-prereqs.sh
bazel fetch -k //...
bazel build //...

echo "=== Go Build ==="
./bin/init.sh

echo "=== Code Check ==="
./bin/check.sh

echo "=== Bazel Tests ==="
bazel test //...
