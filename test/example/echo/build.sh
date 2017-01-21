#!/bin/bash
set -ex
gcloud config set project istio-test

rm -f manager
bazel build //...
cp ../../../bazel-bin/cmd/manager/manager .

docker build -t runtime -f Dockerfile .
docker tag runtime gcr.io/istio-test/runtime:experiment
gcloud docker -- push gcr.io/istio-test/runtime:experiment
