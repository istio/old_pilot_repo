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
# Smoketest script triggered by Prow. #
#######################################

# Exit immediately for non zero status
set -e
# Check unset variables
set -u
# Print commands
set -x

if [ "${CI:-}" == "bootstrap" ]; then
    # Use the provided pull head sha, from prow.
    GIT_SHA=$PULL_PULL_SHA
else
    # Use the current commit.
    GIT_SHA=$(git rev-parse --verify HEAD)
fi

echo "=== Clone istio/istio ==="
rm -rf /tmp/istio
git clone https://github.com/istio/istio /tmp/istio
cd /tmp/istio

HUB="gcr.io/istio-testing"
BUCKET="istio-artifacts"
ISTIOCTL_URL=https://storage.googleapis.com/$BUCKET/pilot/$GIT_SHA/artifacts/istioctl

echo "=== Smoke Test ==="
# Note: These tests use the default ~/.kube/config file. The prow container mounts the test cluster
# kubeconfig at this path. On the other hand, when running this script locally,  this behavior results
# in the test framework using whatever is your current kube context!
#
# In the future, this should be parameterized similarly to the integration tests, with the kubeconfig
# location specified explicitly.
./tests/e2e.sh \
    --logs_bucket_path gs://$BUCKET/pilot/$GIT_SHA/e2e/logs/ \
    --pilot_hub=$HUB \
    --pilot_tag=$GIT_SHA \
    --istioctl_url=$ISTIOCTL_URL

cd -
