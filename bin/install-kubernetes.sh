#!/bin/bash
#
# Copyright 2017 Istio Authors. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
################################################################################
set -x
SCRIPTDIR=$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )

# Need to enable mount propagation.  Older Ubuntu doesn't use
# systemd so need to do it manually
type -a systemctl > /dev/null
if [ $? -ne 0 ]; then
    set -e
    sudo mkdir -p /var/lib/kubelet
    sudo mount --bind /var/lib/kubelet /var/lib/kubelet
    sudo mount --make-shared /var/lib/kubelet
fi

export K8S_VERSION="v1.7.4"
export ARCH=amd64

docker run -d \
    --volume=/:/rootfs:ro \
    --volume=/sys:/sys:ro \
    --volume=/var/lib/docker/:/var/lib/docker:rw \
    --volume=/var/lib/kubelet/:/var/lib/kubelet:rw,shared \
    --volume=/var/run:/var/run:rw \
    --net=host \
    --pid=host \
    --privileged \
    --name=kubelet \
    gcr.io/google_containers/hyperkube-${ARCH}:${K8S_VERSION} \
    /hyperkube kubelet \
        --hostname-override=127.0.0.1 \
        --api-servers=http://localhost:8080 \
        --config=/etc/kubernetes/manifests \
        --allow-privileged --v=2

# Install kubernetes CLI
curl -L http://storage.googleapis.com/kubernetes-release/release/${K8S_VERSION}/bin/linux/${ARCH}/kubectl > /tmp/kubectl
sudo mv /tmp/kubectl /usr/local/bin
sudo chmod +x /usr/local/bin/kubectl
echo "Waiting for K8S to initialize.."
sleep 30
kubectl get po
