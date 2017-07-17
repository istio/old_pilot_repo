#!/bin/bash

# Copyright 2017 Istio Authors

#   Licensed under the Apache License, Version 2.0 (the "License");
#   you may not use this file except in compliance with the License.
#   You may obtain a copy of the License at

#       http://www.apache.org/licenses/LICENSE-2.0

#   Unless required by applicable law or agreed to in writing, software
#   distributed under the License is distributed on an "AS IS" BASIS,
#   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#   See the License for the specific language governing permissions and
#   limitations under the License.


#######################################
# Presubmit script triggered by Prow. #
#######################################

# Exit immediately for non zero status
set -e
# Check unset variables
set -u
# Print commands
set -x

# but we depend on being at path $GOPATH/src/istio.io/pilot for imports
if [ "$CI" == "bootstrap" ]; then
    mkdir -p $GOPATH/src/istio.io
    mv $GOPATH/src/github.com/istio/pilot $GOPATH/src/istio.io
    cd $GOPATH/src/istio.io/pilot/

    # use the provided pull head sha
    GIT_SHA=$PULL_PULL_SHA

    # use volume mount from pilot-presubmit job's pod spec
    ln -s /etc/e2e-testing-kubeconfig/e2e-testing-kubeconfig platform/kube/config
else
    # use the current commit
    GIT_SHA=$(git rev-parse --verify HEAD)
fi

# TODO(nclandolfi) need this line? will remove if not
# gcloud config set container/use_client_certificate True

echo "=== Bazel Build ==="
./bin/install-prereqs.sh
bazel build //...

echo "=== Go Build ==="
./bin/init.sh

echo "=== Code Check ==="
./bin/check.sh

echo "=== Bazel Tests ==="
bazel test //...

echo "=== Running e2e Tests ==="
./bin/e2e.sh -tag $GIT_SHA -hub "gcr.io/istio-testing"
