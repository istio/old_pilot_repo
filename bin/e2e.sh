#!/bin/bash
usage() {
  echo "usage: $0 -h <dockerNamespace> -t <dockerImageTag> -n KubeNamespace"
  echo -e "\tdockerNamespace defaults to $hub"
  echo -e "\tdockerImageTag defaults to $tag"
  echo -e "\tIf namespace is not provided, a random namespace would be generated during the test"
  exit 1
}

# These default values must be consistent with test/integration/driver.go
hub="gcr.io/istio-testing"
tag="$(whoami)"
namespace=""

while getopts :h:t:n: arg; do
  case ${arg} in
    h) hub="${OPTARG}";;
    t) tag="${OPTARG}";;
    n) namespace="${OPTARG}";;
    *) ;;
  esac
done

if [ -z "$hub"  ]; then
    usage
fi

if [ -z "$tag" ]; then
    usage
fi

if [[ "$hub" =~ ^gcr\.io ]]; then
    gcloud docker --authorize-only
fi

set -ex
for image in app init runtime; do
	bazel run //docker:$image
	docker tag istio/docker:$image $hub/$image:$tag
	docker push $hub/$image:$tag
done

declare -a params
params+=("-h" "$hub" "-t" "$tag")
[[ !  -z  "$namespace"  ]] && params+=("-n" "$namespace")
bazel run //test/integration -- "${params[@]}"
