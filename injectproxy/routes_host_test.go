// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package injectproxy

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRewriteHostHeader(t *testing.T) {
	for _, tc := range []struct {
		name        string
		rewriteHost string
		wantHost    string
	}{
		{
			name:        "host header rewritten when option is set",
			rewriteHost: "custom.upstream.example.com",
			wantHost:    "custom.upstream.example.com",
		},
		{
			name:        "original host preserved when option is not set",
			rewriteHost: "",
			wantHost:    "example.com",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var receivedHost, receivedXForwardedFor, receivedXForwardedHost, receivedXForwardedProto string
			m := newMockUpstream(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				receivedHost = req.Host
				receivedXForwardedFor = req.Header.Get("X-Forwarded-For")
				receivedXForwardedHost = req.Header.Get("X-Forwarded-Host")
				receivedXForwardedProto = req.Header.Get("X-Forwarded-Proto")
				w.Write(okResponse)
			}))
			defer m.Close()

			var opts []Option
			if tc.rewriteHost != "" {
				opts = append(opts, WithRewriteHostHeader(tc.rewriteHost))
			}

			r, err := NewRoutes(m.url, proxyLabel, HTTPFormEnforcer{ParameterName: proxyLabel}, opts...)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "http://example.com/api/v1/query?query=up&namespace=test", nil)
			r.ServeHTTP(w, req)

			resp := w.Result()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("expected status code 200, got %d", resp.StatusCode)
			}

			if receivedHost != tc.wantHost {
				t.Errorf("Host: got %q, want %q", receivedHost, tc.wantHost)
			}

			// Verify X-Forwarded-* headers are set by SetXForwarded().
			if receivedXForwardedFor == "" {
				t.Error("X-Forwarded-For header not set on upstream request")
			}
			if receivedXForwardedHost != "example.com" {
				t.Errorf("X-Forwarded-Host: got %q, want %q", receivedXForwardedHost, "example.com")
			}
			if receivedXForwardedProto != "http" {
				t.Errorf("X-Forwarded-Proto: got %q, want %q", receivedXForwardedProto, "http")
			}
		})
	}
}
