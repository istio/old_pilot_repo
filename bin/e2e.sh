#!/bin/bash
set -ex

# These default values must be consistent with test/integration/driver.go
hub="gcr.io/istio-testing"
tag="test"

while getopts :h:t: arg; do
  case ${arg} in
    h) hub="${OPTARG}";;
    t) tag="${OPTARG}";;
    *) ;;
  esac
done

if [[ "$hub" =~ ^gcr\.io ]]; then
    gcloud docker --authorize-only
fi

for image in app init runtime; do
    bazel run //docker:$image
    # bazel strips timestamp from timestamps to make it reproducible
    # (see https://bazel.build/docs/be/docker.html). Use docker
    # save+import so we push images with correct timestamps so k8s
    # image caching and pull policy works correctly.
    docker save istio/docker:$image | docker import - $hub/$image:$tag
    docker push $hub/$image:$tag
done

bazel run //test/integration -- "$@" --norouting
