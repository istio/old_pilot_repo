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

    # Use volume mount from pilot-presubmit job's pod spec.
    ln -s /etc/e2e-testing-kubeconfig/e2e-testing-kubeconfig platform/kube/config
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
./tests/e2e.sh \
    --logs_bucket_path gs://$BUCKET/pilot/$GIT_SHA/e2e/logs/ \
    --pilot_hub=$HUB \
    --pilot_tag=$GIT_SHA \
    --istioctl_url=$ISTIOCTL_URL

cd -
