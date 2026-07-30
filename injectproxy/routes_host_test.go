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
	"fmt"
	"io"
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
			m := newMockUpstream(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if req.Host != tc.wantHost {
					prometheusAPIError(w, fmt.Sprintf("unexpected Host: got %q, want %q", req.Host, tc.wantHost), http.StatusBadRequest)
					return
				}
				if req.Header.Get("X-Forwarded-For") == "" {
					prometheusAPIError(w, "X-Forwarded-For not set", http.StatusBadRequest)
					return
				}
				if got := req.Header.Get("X-Forwarded-Host"); got != "example.com" {
					prometheusAPIError(w, fmt.Sprintf("unexpected X-Forwarded-Host: got %q, want %q", got, "example.com"), http.StatusBadRequest)
					return
				}
				if got := req.Header.Get("X-Forwarded-Proto"); got != "http" {
					prometheusAPIError(w, fmt.Sprintf("unexpected X-Forwarded-Proto: got %q, want %q", got, "http"), http.StatusBadRequest)
					return
				}
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
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("unexpected status %d: %s", resp.StatusCode, body)
			}
		})
	}
}
