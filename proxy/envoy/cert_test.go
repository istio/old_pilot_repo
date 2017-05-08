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

package envoy

import (
	"io/ioutil"
	"os"
	"testing"
	"time"
)

func TestWathCerts(t *testing.T) {
	name, err := ioutil.TempDir("testdata", "certs")
	if err != nil {
		t.Errorf("failed to create a temp dir: %v", err)
	}
	defer os.RemoveAll(name)

	called := make(chan bool)
	callbackFunc := func() {
		called <- true
	}

	stopCh := make(chan struct{})
	go watchCerts(name, stopCh, callbackFunc)

	// sleep one second to make sure the watcher is set up before change is made
	time.Sleep(time.Second)

	// make a change to the watched dir
	if _, err := ioutil.TempFile(name, "test.file"); err != nil {
		t.Errorf("failed to create a temp file in testdata/certs: %v", err)
	}

	select {
	case <-called:
		// expected
	case <-time.After(time.Second):
		t.Errorf("The callback is not called within time limit " + time.Now().String())
	}
}
