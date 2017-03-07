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

// Package version provides build time version information.
package version

// The following fields are populated at buildtime with bazel's linkstamp
// feature. This is equivalent to using golang directly with -ldflags -X.
// Note that DATE is omitted as bazel aims for reproducible builds and
// seems to strip date information from the build process.
var (
	BUILD_APP_VERSION    string
	BUILD_GIT_REVISION   string
	BUILD_GIT_BRANCH     string
	BUILD_USER           string
	BUILD_HOST           string
	BUILD_GOLANG_VERSION string
)

// BuildInfo provides information about the binary build.
type BuildInfo struct {
	Version       string `json:"version"`
	GitRevision   string `json:"revision"`
	GitBranch     string `json:"branch"`
	User          string `json:"user"`
	Host          string `json:"host"`
	GolangVersion string `json:"golang_version"`
}

var Info BuildInfo

func init() {
	Info.Version = BUILD_APP_VERSION
	Info.GitRevision = BUILD_GIT_REVISION
	Info.GitBranch = BUILD_GIT_BRANCH
	Info.User = BUILD_USER
	Info.Host = BUILD_HOST
	Info.GolangVersion = BUILD_GOLANG_VERSION
}
