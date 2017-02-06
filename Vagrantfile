#
# Copyright 2017 IBM Corporation
#
#   Licensed under the Apache License, Version 2.0 (the "License");
#   you may not use this file except in compliance with the License.
#   You may obtain a copy of the License at
#
#       http://www.apache.org/licenses/LICENSE-2.0
#
#   Unless required by applicable law or agreed to in writing, software
#   distributed under the License is distributed on an "AS IS" BASIS,
#   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#   See the License for the specific language governing permissions and
#   limitations under the License.

# This Vagrantfile sets up a bazel based VM for building istio code, while
# sharing the source folder with the host VM. Within the VM, run
# bin/init.sh to generate the go vendor directories. Once done, you can use
# your favorite IDEs in the host machine for hacking on Istio go
# code. However, builds should always be done inside the Vagrant VM.  Make
# sure to setup a separate minikube VM, and ensure that its accessible from
# the bazel VM when running integration tests.

# -*- mode: ruby -*-
# vi: set ft=ruby :

$script = <<SCRIPT
set -x

# Install Docker
sudo apt-get update
sudo apt-key adv --keyserver hkp://p80.pool.sks-keyservers.net:80 --recv-keys 58118E89F3A912897C070ADBF76221572C52609D
sudo apt-add-repository 'deb https://apt.dockerproject.org/repo ubuntu-xenial main'
sudo apt-get update
apt-cache policy docker-engine
sudo apt-get install -y python docker-engine
sudo usermod -a -G docker ubuntu # Add ubuntu user to the docker group
## Install aufs storage driver
sudo apt-get install linux-image-extra-$(uname -r)     linux-image-extra-virtual

# Install Bazel 
echo "deb [arch=amd64] http://storage.googleapis.com/bazel-apt stable jdk1.8" | sudo tee /etc/apt/sources.list.d/bazel.list
curl https://bazel.build/bazel-release.pub.gpg | sudo apt-key add -
sudo apt-get update && sudo apt-get install -y bazel

# Install golang
cd /tmp
curl -O https://storage.googleapis.com/golang/go1.7.5.linux-amd64.tar.gz
tar -C /usr/local -xzf go1.7.5.linux-amd64.tar.gz
if ! grep -Fq "/home/ubuntu/go" /home/ubuntu/.profile; then
	echo 'export GOPATH=/home/ubuntu/go' >> /home/ubuntu/.profile
fi
if ! grep -Fq "/usr/local/go/bin" /home/ubuntu/.profile; then
	echo 'export PATH=$PATH:/usr/local/go/bin:$GOPATH/bin' >> /home/ubuntu/.profile
fi
rm /tmp/go1.7.5.linux-amd64.tar.gz

mkdir -p /home/ubuntu/go/src/istio.io/manager
chown -R ubuntu:ubuntu /home/ubuntu/go

SCRIPT

Vagrant.configure('2') do |config|
  config.vm.box = "ubuntu/xenial64"

  config.vm.synced_folder ".", "/home/ubuntu/go/src/istio.io/manager"
  config.vm.define "istio" do |istio|
    istio.vm.provider :virtualbox do |vb|
      vb.customize ["modifyvm", :id, "--natdnshostresolver1", "on"]
      vb.customize ['modifyvm', :id, '--memory', '4096']
      vb.cpus = 2
    end
  end

  # Port mappings for various services inside the VM

  # Create a private network, which allows vagrant VM to access minikube VM
  Vagrant.configure("2") do |config|
    config.vm.network "private_network", type: "dhcp"
  end

  config.vm.provision :shell, inline: $script
end
