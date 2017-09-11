#!/bin/bash

# Example usage:
#
# bin/push-debian.sh \
#   -c opt
#   -v 0.2.1

function usage() {
  echo "$0 \
    -c <bazel config to use> \
    -v <istio version number>"
  exit 1
}

while getopts ":c:v:" arg; do
  case ${arg} in
    c) BAZEL_ARGS="--config=${OPTARG}";;
    v) ISTIO_VERSION="${OPTARG}";;
    *) usage;;
  esac
done

if [ -z "${BAZEL_ARGS}" ] || [ -z "${ISTIO_VERSION}" ]; then
  usage
fi

set -ex

bazel ${BAZEL_STARTUP_ARGS} build ${BAZEL_ARGS} "//tools/deb:istio-agent"
gsutil -m cp -r \
  bazel-bin/tools/deb/istio-agent_${ISTIO_VERSION}_amd64.* \
  gs://istio-release/releases/${ISTIO_VERSION}/deb
