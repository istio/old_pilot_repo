// Copyright 2017 Google Inc.
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

// Functions related to header configuration in envoy: match conditions

package envoy

import (
	"fmt"
	"sort"

	"github.com/golang/glog"

	"istio.io/manager/model/proxy/alphav1/config"
)

// TODO: test sorting, translation

// buildHTTPRoute creates an HTTP route for the route rule condition
func buildHTTPRoute(match *config.MatchCondition) *Route {
	path := ""
	prefix := "/"

	if uri, ok := match.Http[HeaderURI]; ok {
		switch m := uri.MatchType.(type) {
		case *config.StringMatch_Exact:
			path = m.Exact
			prefix = ""
		case *config.StringMatch_Prefix:
			path = ""
			prefix = m.Prefix
		case *config.StringMatch_Regex:
			glog.Warningf("Unsupported route match condition: regex")
			return nil
		}
	}

	return &Route{
		Headers: buildHeaders(match.Http),
		Path:    path,
		Prefix:  prefix,
	}
}

// buildHeaders skips over URI as it has special meaning
func buildHeaders(matches map[string]*config.StringMatch) []Header {
	headers := make([]Header, 0, len(matches))
	for name, match := range matches {
		if name != HeaderURI {
			headers = append(headers, buildHeader(name, match))
		}
	}
	sort.Sort(Headers(headers))
	return headers
}

func buildHeader(name string, match *config.StringMatch) Header {
	value := ""
	regex := false

	switch m := match.MatchType.(type) {
	case *config.StringMatch_Exact:
		value = m.Exact
	case *config.StringMatch_Prefix:
		// TODO(rshriram): escape prefix string into regex, define regex standard
		value = fmt.Sprintf("^%v.*", m.Prefix)
		regex = true
	case *config.StringMatch_Regex:
		value = m.Regex
		regex = true
	default:
		glog.Warningf("Missing header match type, defaulting to empty value: %#v", match.MatchType)
	}

	return Header{
		Name:  name,
		Value: value,
		Regex: regex,
	}
}
