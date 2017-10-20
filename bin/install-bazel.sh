#!/bin/bash

set -e

# Bazel and related dependencies.
#apt-get install -y openjdk-8-jdk curl
echo "deb [arch=amd64] http://storage.googleapis.com/bazel-apt stable jdk1.8" >bazel.list
sudo mv bazel.list /etc/apt/sources.list.d/bazel.list
curl https://bazel.build/bazel-release.pub.gpg | sudo apt-key add -
sudo apt-get update
sudo apt-get install -y bazel
