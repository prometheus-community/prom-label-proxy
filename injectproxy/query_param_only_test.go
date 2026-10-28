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
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestHTTPFormEnforcerQueryOnly(t *testing.T) {
	t.Parallel()

	const param = "namespace"

	tests := []struct {
		name       string
		method     string
		urlNS      []string
		formNS     []string
		formQuery  string
		wantCode   int
		wantLabels []string
	}{
		{
			name:       "GET with URL namespace",
			method:     http.MethodGet,
			urlNS:      []string{"team-a"},
			wantCode:   http.StatusOK,
			wantLabels: []string{"team-a"},
		},
		{
			name:       "GET with repeated URL namespaces",
			method:     http.MethodGet,
			urlNS:      []string{"team-a", "team-b"},
			wantCode:   http.StatusOK,
			wantLabels: []string{"team-a", "team-b"},
		},
		{
			name:     "GET missing URL namespace",
			method:   http.MethodGet,
			wantCode: http.StatusBadRequest,
		},
		{
			name:       "POST query in form, namespace in URL only",
			method:     http.MethodPost,
			urlNS:      []string{"team-a"},
			formQuery:  "up",
			wantCode:   http.StatusOK,
			wantLabels: []string{"team-a"},
		},
		{
			name:       "POST query in form, repeated URL namespaces",
			method:     http.MethodPost,
			urlNS:      []string{"team-a", "team-b"},
			formQuery:  "up",
			wantCode:   http.StatusOK,
			wantLabels: []string{"team-a", "team-b"},
		},
		{
			name:      "POST form namespace rejected when URL has authorized namespace",
			method:    http.MethodPost,
			urlNS:     []string{"team-a"},
			formNS:    []string{"team-b"},
			formQuery: "up",
			wantCode:  http.StatusBadRequest,
		},
		{
			name:      "POST same namespace in URL and form body still rejected",
			method:    http.MethodPost,
			urlNS:     []string{"team-a"},
			formNS:    []string{"team-a"},
			formQuery: "up",
			wantCode:  http.StatusBadRequest,
		},
		{
			name:      "POST form-only namespace rejected",
			method:    http.MethodPost,
			formNS:    []string{"team-a"},
			formQuery: "up",
			wantCode:  http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var gotLabels []string
			var gotQuery string
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotLabels = MustLabelValues(r.Context())
				// Canary: ParseForm in QueryOnly must leave PostForm readable for downstream.
				if err := r.ParseForm(); err == nil {
					gotQuery = r.PostForm.Get("query")
				}
				w.WriteHeader(http.StatusOK)
			})
			handler := HTTPFormEnforcer{ParameterName: param, QueryOnly: true}.ExtractLabel(next)

			u := &url.URL{Path: "/api/v1/query"}
			q := u.Query()
			for _, ns := range tc.urlNS {
				q.Add(param, ns)
			}
			u.RawQuery = q.Encode()

			var body io.Reader
			if tc.method == http.MethodPost {
				form := url.Values{}
				for _, ns := range tc.formNS {
					form.Add(param, ns)
				}
				if tc.formQuery != "" {
					form.Set("query", tc.formQuery)
				}
				body = strings.NewReader(form.Encode())
			}

			req := httptest.NewRequest(tc.method, u.String(), body)
			if tc.method == http.MethodPost {
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tc.wantCode {
				t.Fatalf("status=%d want=%d body=%q", rec.Code, tc.wantCode, rec.Body.String())
			}
			if tc.wantCode != http.StatusOK {
				return
			}
			if len(gotLabels) != len(tc.wantLabels) {
				t.Fatalf("labels=%v want=%v", gotLabels, tc.wantLabels)
			}
			for i := range tc.wantLabels {
				if gotLabels[i] != tc.wantLabels[i] {
					t.Fatalf("labels=%v want=%v", gotLabels, tc.wantLabels)
				}
			}
			if tc.formQuery != "" && gotQuery != tc.formQuery {
				t.Fatalf("post-form query=%q want=%q (body data lost after ParseForm)", gotQuery, tc.formQuery)
			}
		})
	}
}

func TestHTTPFormEnforcerDefaultStillMergesForm(t *testing.T) {
	t.Parallel()

	const param = "namespace"
	var gotLabels []string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotLabels = MustLabelValues(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	handler := HTTPFormEnforcer{ParameterName: param}.ExtractLabel(next)

	form := url.Values{}
	form.Set(param, "from-form")
	form.Set("query", "up")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/query?namespace=from-url", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	// r.Form after ParseForm merges PostForm then URL query.
	want := []string{"from-form", "from-url"}
	if len(gotLabels) != len(want) {
		t.Fatalf("labels=%v want=%v", gotLabels, want)
	}
	for i := range want {
		if gotLabels[i] != want[i] {
			t.Fatalf("labels=%v want=%v", gotLabels, want)
		}
	}
}
