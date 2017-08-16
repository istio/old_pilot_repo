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
#
# Generate structs for CRDs

set -ex
set -o errexit
set -o nounset
set -o pipefail

cat > adapter/config/crd/registry.go << EOF
// Copyright 2017 Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package crd

import (
	"istio.io/pilot/model"
	"k8s.io/apimachinery/pkg/runtime"
)

var knownTypes = map[string]struct {
	obj        runtime.Object
	collection runtime.Object
}{
EOF

for crd in Mock RouteRule IngressRule DestinationPolicy; do
  sed -e s/IstioKind/$crd/g adapter/config/crd/config.go > adapter/config/crd/$crd.go
cat >> adapter/config/crd/registry.go << EOF
	model.$crd.Type: {
		obj:        &${crd}{},
		collection: &${crd}List{},
	},
EOF
done

cat >> adapter/config/crd/registry.go <<EOF
}
EOF
