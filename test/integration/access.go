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

import "sync"

// envoy access log testing utilities

// accessLogs collects test expectations for access logs
type accessLogs struct {
	mu sync.Mutex

	// logs is a mapping from app name to a collection of request IDs keyed by unique description text
	logs map[string]map[string]string
}

func makeAccessLogs() *accessLogs {
	return &accessLogs{
		logs: make(map[string]map[string]string),
	}
}

func (a *accessLogs) add(app, desc, id) {
	a.mu.Lock()
	defer a.mu.Unlock()
}
