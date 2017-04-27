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
	for i := 0; i<120; i++ {
		request, err := util.Shell(fmt.Sprintf("kubectl exec %s -n %s -c app -- client -url http://%s",
			t.apps["a"][0], t.Namespace, "b"))
		if err != nil {
			return err
		}
		fmt.Println(request)
		time.Sleep(1 * time.Second)
	}
	time.Sleep(1 * time.Hour)
	return nil
}

func (t *zipkin) teardown() {
}
