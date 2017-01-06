#!/bin/bash
gcloud config set project istio-test

# Expects "mixs" binary
docker build -t mixer -f Dockerfile-mixer .
docker tag mixer gcr.io/istio-test/mixer:example
gcloud docker -- push gcr.io/istio-test/mixer:example

# Expects "envoy_esp" binary
docker build -t envoy -f Dockerfile-envoy .
docker tag envoy gcr.io/istio-test/envoy:example
gcloud docker -- push gcr.io/istio-test/envoy:example

