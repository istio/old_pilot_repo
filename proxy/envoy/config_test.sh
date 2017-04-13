#!/bin/bash
# Check that envoy can parse golden configuration artifacts

set -o errexit
set -o nounset
set -o pipefail

#proxy/envoy/envoy -l trace -c proxy/envoy/testdata/envoy-v0.json.golden --service-cluster istio-proxy --service-node 10.0.0.1
