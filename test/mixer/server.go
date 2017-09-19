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

package mixer

import (
	"golang.org/x/net/context"

	"istio.io/pilot/test/mixer/pb"
)

// Server is a basic Mixer server
type Server struct {
	counter int
}

func (s *Server) Check(context.Context, *pb.CheckRequest) (*pb.CheckResponse, error) {
	s.counter++
	return nil, nil
}

func (s *Server) Report(context.Context, *pb.ReportRequest) (*pb.ReportResponse, error) {
	// do nothing deliberately
	return &pb.ReportReponse{}, nil
}
