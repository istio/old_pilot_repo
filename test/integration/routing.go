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

// Routing tests

package main

/*
func testRouting() error {
	// First test default routing
	// Create a bytes buffer to hold the YAML form of rules
	glog.Info("Routing all traffic to world-v1 and verifying..")
	deployDynamicConfig("rule-default-route.yaml.tmpl", map[string]string{
		"destination": "c",
		"Namespace":   params.Namespace,
	}, model.RouteRule, "default-route", "a")
	check(verifyRouting("a", "c", "", "",
		100, map[string]int{
			"v1": 100,
			"v2": 0,
		}))
	glog.Info("Success!")

	glog.Info("Routing 75 percent to world-v1, 25 percent to world-v2 and verifying..")
	deployDynamicConfig("rule-weighted-route.yaml.tmpl", map[string]string{
		"destination": "c",
		"Namespace":   params.Namespace,
	}, model.RouteRule, "default-route", "a")
	check(verifyRouting("a", "c", "", "",
		100, map[string]int{
			"v1": 75,
			"v2": 25,
		}))
	glog.Info("Success!")

	glog.Info("Routing 100 percent to world-v2 using header based routing and verifying..")
	deployDynamicConfig("rule-content-route.yaml.tmpl", map[string]string{
		"source":      "a",
		"destination": "c",
		"Namespace":   params.Namespace,
	}, model.RouteRule, "content-route", "a")
	check(verifyRouting("a", "c", "version", "v2",
		100, map[string]int{
			"v1": 0,
			"v2": 100,
		}))
	glog.Info("Success!")

	glog.Info("Testing fault injection..")
	deployDynamicConfig("rule-fault-injection.yaml.tmpl", map[string]string{
		"source":      "a",
		"destination": "c",
		"Namespace":   params.Namespace,
	}, model.RouteRule, "fault-injection", "a")
	check(verifyFaultInjection("a", "c", "version", "v2", time.Second*5, 503))
	glog.Info("Success!")

	glog.Info("Cleaning up route rules...")
	check(util.Run("kubectl delete istioconfigs --all -n " + params.Namespace))

	return nil
}

// verifyRouting verifies if the traffic is split as specified across different deployments in a service
func verifyRouting(src, dst, headerKey, headerVal string, samples int, expectedCount map[string]int) error {
	count := make(map[string]int)
	for version := range expectedCount {
		count[version] = 0
	}

	url := fmt.Sprintf("http://%s/%s", dst, src)
	glog.Infof("Making %d requests (%s) from %s...\n", samples, url, src)

	cmd := fmt.Sprintf("kubectl exec %s -n %s -c app -- client -url %s -count %d -key %s -val %s",
		apps[src][0], params.Namespace, url, samples, headerKey, headerVal)
	request, err := util.Shell(cmd)
	glog.V(2).Info(request)
	if err != nil {
		return err
	}

	matches := regexp.MustCompile("ServiceVersion=(.*)").FindAllStringSubmatch(request, -1)
	for _, match := range matches {
		if len(match) > 1 {
			id := match[1]
			count[id]++
		}
	}

	epsilon := 5

	var failures int
	for version, expected := range expectedCount {
		if count[version] > expected+epsilon || count[version] < expected-epsilon {
			glog.Infof("Expected %v requests (+/-%v) to reach %s => Got %v\n", expected, epsilon, version, count[version])
			failures++
		}
	}

	if failures > 0 {
		glog.Info(request)
		return errors.New("routing verification failed")
	}
	return nil
}

// verifyFaultInjection verifies if the fault filter was setup properly
func verifyFaultInjection(src, dst, headerKey, headerVal string,
	respTime time.Duration, respCode int) error {

	url := fmt.Sprintf("http://%s/%s", dst, src)
	glog.Infof("Making 1 request (%s) from %s...\n", url, src)
	cmd := fmt.Sprintf("kubectl exec %s -n %s -c app -- client -url %s -key %s -val %s",
		apps[src][0], params.Namespace, url, headerKey, headerVal)

	start := time.Now()
	request, err := util.Shell(cmd)
	glog.V(2).Info(request)
	elapsed := time.Since(start)
	if err != nil {
		return err
	}

	match := regexp.MustCompile("StatusCode=(.*)").FindStringSubmatch(request)
	statusCode := 0
	if len(match) > 1 {
		statusCode, err = strconv.Atoi(match[1])
		if err != nil {
			statusCode = -1
		}
	}

	// +/- 1s variance
	epsilon := time.Second * 2
	glog.Infof("Response time is %s with status code %d\n", elapsed, statusCode)
	glog.Infof("Expected response time is %s +/- %s with status code %d\n", respTime, epsilon, respCode)
	if elapsed > respTime+epsilon || elapsed < respTime-epsilon || respCode != statusCode {
		return errors.New("fault injection verification failed")
	}
	return nil
}
*/
