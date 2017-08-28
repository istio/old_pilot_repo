#!/bin/bash

set -e

if [ ! -f platform/kube/config ]; then
  ln -s ~/.kube/config platform/kube/
fi
