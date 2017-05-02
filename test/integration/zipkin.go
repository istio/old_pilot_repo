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

package main

import (
	"time"
	"istio.io/manager/test/util"
	"fmt"
	"github.com/satori/go.uuid"
)

type zipkin struct {
	*infra
}

func (t *zipkin) String() string {
	return "zipkin"
}

func (t *zipkin) setup() error {
	return nil
}

func (t *zipkin) run() error {

	for i := 0; i<5; i++ {
		id := uuid.NewV4()
		request, err := util.Shell(fmt.Sprintf("kubectl exec %s -n %s -c app -- client -url http://%s -key %v -val %v",
			t.apps["a"][0], t.Namespace, "b", "x-client-trace-id", id))
		if err != nil {
			return err
		}
		fmt.Println(request) // TODO: remove me
		time.Sleep(1 * time.Second)
	}

	// TODO: verify that the traces are reaching Zipkin
	// To verify:
	// curl http://192.168.99.100:30703/api/v1/traces

	return nil
}

func (t *zipkin) teardown() {
}
