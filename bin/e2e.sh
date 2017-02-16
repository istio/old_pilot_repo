#!/bin/bash
set -ex

# These default values must be consistent with test/integration/driver.go
hub="gcr.io/istio-testing"
tag=$(whoami)_$(date +%Y%m%d_%H%M%S)

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
    docker tag istio/docker:$image $hub/$image:$tag
    docker push $hub/$image:$tag
done

bazel run //test/integration -- "$@" --norouting
