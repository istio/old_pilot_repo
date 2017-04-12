package client

import (
	"net/url"
	"testing"
)

func TestNewManagerClient(t *testing.T) {

	cases := []struct {
		name          string
		baseURL       string
		expBaseURL    string
		apiVersion    string
		expAPIVersion string
	}{
		{
			name:       "TestBaseURL",
			baseURL:    "http://host:port",
			expBaseURL: "http://host:port",
		},
		{
			name:       "TestBaseURLTrailingSlash",
			baseURL:    "http://host:port/",
			expBaseURL: "http://host:port",
		},
		{
			name:          "TestAPIVersion",
			apiVersion:    "test",
			expAPIVersion: "test",
		},
		{
			name:          "TestAPIVersionTrailingSlash",
			apiVersion:    "test/",
			expAPIVersion: "test",
		},
		{
			name:          "TestAPIVersionLeadingSlash",
			apiVersion:    "/test",
			expAPIVersion: "test",
		},
	}

	for _, c := range cases {
		url, _ := url.Parse(c.baseURL)
		client := NewManagerClient(url, c.apiVersion, nil)
		if c.baseURL != "" && client.base.String() != c.expBaseURL {
			t.Errorf("%s: got baseURL %v, want %v", c.name, client.base.String(), c.expBaseURL)
		}
		if c.apiVersion != "" && client.versionedAPIPath != c.expAPIVersion {
			t.Errorf("%s: got apiVersion %v, want %v", c.name, client.versionedAPIPath, c.expAPIVersion)
		}
	}
}
