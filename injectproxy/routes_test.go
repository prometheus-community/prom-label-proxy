// Copyright 2020 The Prometheus Authors
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
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"
)

var okResponse = []byte(`ok`)

func checkParameterAbsent(param string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		kvs, err := url.ParseQuery(req.URL.RawQuery)
		if err != nil {
			http.Error(w, fmt.Sprintf("unexpected error: %v", err), http.StatusInternalServerError)
			return
		}
		if len(kvs[param]) != 0 {
			http.Error(w, fmt.Sprintf("unexpected parameter %q", param), http.StatusInternalServerError)
			return
		}
		next.ServeHTTP(w, req)
	})
}

// checkQueryParameterHandler verifies that the request contains the given parameter key/values.
func checkQueryParameterHandler(key string, values ...string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		kvs, err := url.ParseQuery(req.URL.RawQuery)
		if err != nil {
			http.Error(w, fmt.Sprintf("unexpected error: %v", err), http.StatusInternalServerError)
			return
		}
		// Verify that the client provides the parameter only once.
		if len(kvs[key]) != len(values) {
			http.Error(w, fmt.Sprintf("expected %d values of parameter %q, got %d", len(values), key, len(kvs[key])), http.StatusInternalServerError)
			return
		}
		sort.Strings(values)
		sort.Strings(kvs[key])
		for i := range values {
			if kvs[key][i] != values[i] {
				http.Error(w, fmt.Sprintf("expected parameter %q with value %q, got %q", key, values[i], kvs[key][i]), http.StatusInternalServerError)
				return
			}
		}
		w.Write(okResponse)
	})
}

// mockUpstream simulates an upstream HTTP server. It runs on localhost.
type mockUpstream struct {
	h   http.Handler
	srv *httptest.Server
	url *url.URL
}

func newMockUpstream(h http.Handler) *mockUpstream {
	m := mockUpstream{h: h}

	m.srv = httptest.NewServer(&m)

	u, err := url.Parse(m.srv.URL)
	if err != nil {
		panic(err)
	}
	m.url = u

	return &m
}

func (m *mockUpstream) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	m.h.ServeHTTP(w, req)
}

func (m *mockUpstream) Close() {
	m.srv.Close()
}

const proxyLabel = "namespace"

func TestEndpointNotImplemented(t *testing.T) {
	m := newMockUpstream(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Write(okResponse)
	}))
	defer m.Close()
	r := NewRoutes(m.url, proxyLabel)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://prometheus.example.com/graph?namespace=ns1", nil)

	r.ServeHTTP(w, req)
	resp := w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected status code 404, got %d", resp.StatusCode)
	}
}

func TestMatch(t *testing.T) {
	for _, tc := range []struct {
		labelv  string
		matches []string

		expCode  int
		expMatch string
		expBody  []byte
	}{
		{
			// No "namespace" parameter returns an error.
			expCode: http.StatusBadRequest,
		},
		{
			// No "match" parameter.
			labelv:   "default",
			expCode:  http.StatusOK,
			expMatch: `{namespace="default"}`,
			expBody:  okResponse,
		},
		{
			// Many "match" parameters.
			labelv:   "default",
			matches:  []string{`{job="prometheus"}`, `{__name__=~"job:.*"}`},
			expCode:  http.StatusOK,
			expMatch: `{namespace="default"}`,
			expBody:  okResponse,
		},
	} {
		for _, u := range []string{
			"http://prometheus.example.com/federate",
			"http://prometheus.example.com/api/v1/labels",
			"http://prometheus.example.com/api/v1/label/some_label/values",
		} {
			t.Run(fmt.Sprintf("%s?match[]=%s", u, strings.Join(tc.matches, "&")), func(t *testing.T) {
				m := newMockUpstream(
					checkParameterAbsent(
						proxyLabel,
						checkQueryParameterHandler("match[]", tc.expMatch),
					),
				)
				defer m.Close()
				r := NewRoutes(m.url, proxyLabel, WithEnabledLabelsAPI())

				u, err := url.Parse(u)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				q := u.Query()
				for _, m := range tc.matches {
					q.Add("match[]", m)
				}
				q.Set(proxyLabel, tc.labelv)
				u.RawQuery = q.Encode()

				w := httptest.NewRecorder()
				req := httptest.NewRequest("GET", u.String(), nil)
				r.ServeHTTP(w, req)

				resp := w.Result()
				body, _ := ioutil.ReadAll(resp.Body)
				defer resp.Body.Close()

				if resp.StatusCode != tc.expCode {
					t.Logf("expected status code %d, got %d", tc.expCode, resp.StatusCode)
					t.Logf("%s", string(body))
					t.FailNow()
				}
				if resp.StatusCode != http.StatusOK {
					return
				}

				if string(body) != string(tc.expBody) {
					t.Fatalf("expected body %q, got %q", string(tc.expBody), string(body))
				}
			})
		}
	}
}

func TestQuery(t *testing.T) {
	for _, tc := range []struct {
		labelv    string
		promQuery string

		expCode      int
		expPromQuery string
		expBody      []byte
	}{
		{
			// No "namespace" parameter returns an error.
			expCode: http.StatusBadRequest,
		},
		{
			// No "query" parameter returns 200 with empty body.
			labelv:  "default",
			expCode: http.StatusOK,
		},
		{
			// Query without a vector selector.
			labelv:       "default",
			promQuery:    "up",
			expCode:      http.StatusOK,
			expPromQuery: `up{namespace="default"}`,
			expBody:      okResponse,
		},
		{
			// Query with a vector selector.
			labelv:       "default",
			promQuery:    `up{namespace="other"}`,
			expCode:      http.StatusOK,
			expPromQuery: `up{namespace="default"}`,
			expBody:      okResponse,
		},
		{
			// Query with a vector selector.
			labelv:       "default",
			promQuery:    `up{namespace="default",}`,
			expCode:      http.StatusOK,
			expPromQuery: `up{namespace="default"}`,
			expBody:      okResponse,
		},
		{
			// Query with a scalar.
			labelv:       "default",
			promQuery:    "1",
			expCode:      http.StatusOK,
			expPromQuery: `1`,
			expBody:      okResponse,
		},
		{
			// Query with a function.
			labelv:       "default",
			promQuery:    "time()",
			expCode:      http.StatusOK,
			expPromQuery: `time()`,
			expBody:      okResponse,
		},
		{
			// An invalid expression returns 200 with empty body.
			labelv:    "default",
			promQuery: "up +",
			expCode:   http.StatusOK,
		},
	} {
		for _, endpoint := range []string{"query", "query_range"} {
			t.Run(endpoint+"/"+tc.promQuery, func(t *testing.T) {
				m := newMockUpstream(
					checkParameterAbsent(
						proxyLabel,
						checkQueryParameterHandler("query", tc.expPromQuery),
					),
				)
				defer m.Close()
				r := NewRoutes(m.url, proxyLabel)

				u, err := url.Parse("http://prometheus.example.com/api/v1/" + endpoint)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				q := u.Query()
				q.Set("query", tc.promQuery)
				q.Set(proxyLabel, tc.labelv)
				u.RawQuery = q.Encode()

				w := httptest.NewRecorder()
				req := httptest.NewRequest("GET", u.String(), nil)
				r.ServeHTTP(w, req)

				resp := w.Result()
				body, _ := ioutil.ReadAll(resp.Body)
				defer resp.Body.Close()

				if resp.StatusCode != tc.expCode {
					t.Logf("expected status code %d, got %d", tc.expCode, resp.StatusCode)
					t.Logf("%s", string(body))
					t.FailNow()
				}
				if resp.StatusCode != http.StatusOK {
					return
				}
				if string(body) != string(tc.expBody) {
					t.Fatalf("expected body %q, got %q", string(tc.expBody), string(body))
				}
			})
		}
	}
}
