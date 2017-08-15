#!/bin/bash
set -ex

bazel fetch @com_github_istio_test_infra//toolbox/pkg_check \
  || { echo 'Failed to bazel fetch pkg_check'; exit 0; }

PILOT_PATH=$(pwd)
BAZEL_OUTPUT_BASE=$(bazel info | grep output_base)
BAZEL_CACHE_PATH=${BAZEL_OUTPUT_BASE#*output_base:}
TEST_INFRA_PATH=${BAZEL_CACHE_PATH}/external/com_github_istio_test_infra

cd ${TEST_INFRA_PATH}
git submodule init \
  || { echo 'Failed to initalize submodule of test-infra'; exit 0; }
git submodule update \
  || { echo 'Failed to update submodule of test-infra'; exit 0; }

cd ${PILOT_PATH}
go run ${TEST_INFRA_PATH}/toolbox/pkg_check/main.go
