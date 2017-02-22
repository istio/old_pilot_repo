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

// Reachability tests

package main

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/golang/sync/errgroup"
)

var (
	// accessLogs is a mapping from app name to a list of request ids that should be present in it
	accessLogs map[string][]string
	accessMu   sync.Mutex
)

func testBasicReachability() error {
	log.Printf("Verifying basic reachability across pods/services (a, b, and t)..")

	accessLogs = make(map[string][]string)
	for app := range pods {
		accessLogs[app] = make([]string, 0)
	}

	err := makeRequests()
	if err != nil {
		return err
	}
	if verbose {
		log.Println("requests:", accessLogs)
	}

	err = checkAccessLogs(accessLogs)
	if err != nil {
		return err
	}
	log.Println("Success!")
	return nil
}

// makeRequest creates a function to make requests; done should return true to quickly exit the retry loop
func makeRequest(src, dst, port, domain string, done func() bool) func() error {
	return func() error {
		url := fmt.Sprintf("http://%s%s%s/%s", dst, domain, port, src)
		for n := 0; n < budget; n++ {
			log.Printf("Making a request %s from %s (attempt %d)...\n", url, src, n)

			request, err := shell(fmt.Sprintf("kubectl exec %s -n %s -c app client %s", pods[src], namespace, url), verbose)
			if err != nil {
				return err
			}
			if verbose {
				log.Println(request)
			}
			match := regexp.MustCompile("X-Request-Id=(.*)").FindStringSubmatch(request)
			if len(match) > 1 {
				id := match[1]
				if verbose {
					log.Printf("id=%s\n", id)
				}
				accessMu.Lock()
				accessLogs[src] = append(accessLogs[src], id)
				accessLogs[dst] = append(accessLogs[dst], id)
				accessMu.Unlock()
				return nil
			}

			// Expected no match
			if src == "t" && dst == "t" {
				if verbose {
					log.Println("Expected no match for t->t")
				}
				return nil
			}
			if done() {
				return nil
			}
		}
		return fmt.Errorf("failed to inject proxy from %s to %s (url %s)", src, dst, url)
	}
}

// makeRequests executes requests in pods and collects request ids per pod to check against access logs
func makeRequests() error {
	log.Printf("makeRequests parallel=%t\n", parallel)
	g, ctx := errgroup.WithContext(context.Background())
	testPods := []string{"a", "b", "t"}
	for _, src := range testPods {
		for _, dst := range testPods {
			for _, port := range []string{"", ":80", ":8080"} {
				for _, domain := range []string{"", "." + namespace} {
					if parallel {
						g.Go(makeRequest(src, dst, port, domain, func() bool {
							select {
							case <-time.After(time.Second):
								// try again
							case <-ctx.Done():
								return true
							}
							return false
						}))
					} else {
						if err := makeRequest(src, dst, port, domain, func() bool { return false })(); err != nil {
							return err
						}
					}
				}
			}
		}
	}
	if parallel {
		if err := g.Wait(); err != nil {
			return err
		}
	}
	return nil
}

func checkAccessLogs(accessLogs map[string][]string) error {
	log.Println("Checking access logs of pods to correlate request IDs...")
	for n := 0; ; n++ {
		found := true
		for _, pod := range []string{"a", "b"} {
			if verbose {
				log.Printf("Checking access log of %s\n", pod)
			}
			access := podLogs(pods[pod], "proxy")
			for _, id := range accessLogs[pod] {
				if !strings.Contains(access, id) {
					if verbose {
						log.Printf("Failed to find request id %s in log of %s\n", id, pod)
					}
					found = false
					break
				}
			}
			if !found {
				break
			}
		}

		if found {
			return nil
		}

		if n > budget {
			return fmt.Errorf("exceeded budget for checking access logs")
		}

		time.Sleep(time.Second)
	}
}
