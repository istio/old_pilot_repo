#!/bin/bash

set -e

# Bazel and related dependencies.
#apt-get install -y openjdk-8-jdk curl
echo "deb [arch=amd64] http://storage.googleapis.com/bazel-apt stable jdk1.8" >extras.list
echo "deb http://apt.kubernetes.io/ kubernetes-xenial main" >>extras.list
sudo mv extras.list /etc/apt/sources.list.d/extras.list
curl https://bazel.build/bazel-release.pub.gpg | sudo apt-key add -
curl -s https://packages.cloud.google.com/apt/doc/apt-key.gpg | sudo apt-key add -

sudo apt-get update
sudo apt-get install -y bazel socat apt-transport-https kubelet kubeadm kubectl
