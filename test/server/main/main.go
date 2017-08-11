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

// An example implementation of an echo backend.
//
// To test flaky HTTP service recovery, this backend has a special features.
// If the "?codes=" query parameter is used it will return HTTP response codes other than 200
// according to a probability distribution.
// For example, ?codes=500:90,200:10 returns 500 90% of times and 200 10% of times
// For example, ?codes=500:1,200:1 returns 500 50% of times and 200 50% of times
// For example, ?codes=501:999,401:1 returns 500 99.9% of times and 401 0.1% of times.
// For example, ?codes=500,200 returns 500 50% of times and 200 50% of times

package main

import (
	"os"
	"os/signal"
	"syscall"

	flag "github.com/spf13/pflag"
	"istio.io/pilot/test/server"
)

var (
	ports     []int
	grpcPorts []int
	version   string

	crt, key string
)

func init() {
	flag.IntSliceVar(&ports, "port", []int{8080}, "HTTP/1.1 ports")
	flag.IntSliceVar(&grpcPorts, "grpc", []int{7070}, "GRPC ports")
	flag.StringVar(&version, "version", "", "Version string")
	flag.StringVar(&crt, "crt", "", "gRPC TLS server-side certificate")
	flag.StringVar(&key, "key", "", "gRPC TLS server-side key")
}

func main() {
	flag.Parse()
	for _, port := range ports {
		go server.RunHTTP(port, version)
	}
	for _, grpcPort := range grpcPorts {
		go server.RunGRPC(grpcPort, version, crt, key)
	}
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs
}
