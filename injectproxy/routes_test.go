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
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"
)

var okResponse = []byte(`ok`)

func checkParameterAbsent(param string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		kvs, err := url.ParseQuery(req.URL.RawQuery)
		if err != nil {
			prometheusAPIError(w, fmt.Sprintf("unexpected error: %v", err), http.StatusInternalServerError)
			return
		}
		if len(kvs[param]) != 0 {
			prometheusAPIError(w, fmt.Sprintf("unexpected parameter %q", param), http.StatusInternalServerError)
			return
		}
		next.ServeHTTP(w, req)
	})
}

func checkFormParameterAbsent(param string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		err := req.ParseForm()
		if err != nil {
			prometheusAPIError(w, fmt.Sprintf("unexpected error: %v", err), http.StatusInternalServerError)
			return
		}
		kvs := req.Form
		if len(kvs[param]) != 0 {
			prometheusAPIError(w, fmt.Sprintf("unexpected Form parameter %q", param), http.StatusInternalServerError)
			return
		}
		next.ServeHTTP(w, req)
	})
}

// checkQueryHandler verifies that the request form contains the given parameter key/values.
func checkQueryHandler(body, key string, values ...string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		kvs, err := url.ParseQuery(req.URL.RawQuery)
		if err != nil {
			prometheusAPIError(w, fmt.Sprintf("unexpected error: %v", err), http.StatusInternalServerError)
			return
		}
		// Verify that the client provides the parameter only once.
		if len(kvs[key]) != len(values) {
			prometheusAPIError(w, fmt.Sprintf("expected %d values of parameter %q, got %d", len(values), key, len(kvs[key])), http.StatusInternalServerError)
			return
		}
		sort.Strings(values)
		sort.Strings(kvs[key])
		for i := range values {
			if kvs[key][i] != values[i] {
				prometheusAPIError(w, fmt.Sprintf("expected parameter %q with value %q, got %q", key, values[i], kvs[key][i]), http.StatusInternalServerError)
				return
			}
		}
		buf, err := ioutil.ReadAll(req.Body)
		if err != nil {
			prometheusAPIError(w, "failed to read body", http.StatusInternalServerError)
			return
		}
		if string(buf) != body {
			prometheusAPIError(w, fmt.Sprintf("expected body %q, got %q", body, string(buf)), http.StatusInternalServerError)
			return
		}

		w.Write(okResponse)
		<-time.After(100)
	})
}

// checkFormHandler verifies that the request Form contains the given parameter key/values.
func checkFormHandler(key string, values ...string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		err := req.ParseForm()
		if err != nil {
			prometheusAPIError(w, fmt.Sprintf("unexpected error: %v", err), http.StatusInternalServerError)
			return
		}
		kvs := req.PostForm
		// Verify that the client provides the parameter only once.
		if len(kvs[key]) != len(values) {
			prometheusAPIError(w, fmt.Sprintf("expected %d values of parameter %q, got %d", len(values), key, len(kvs[key])), http.StatusInternalServerError)
			return
		}
		sort.Strings(values)
		sort.Strings(kvs[key])
		for i := range values {
			if kvs[key][i] != values[i] {
				prometheusAPIError(w, fmt.Sprintf("expected parameter %q with value %q, got %q", key, values[i], kvs[key][i]), http.StatusInternalServerError)
				return
			}
		}
		w.Write(okResponse)
		<-time.After(100)
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

func TestWithPassthroughPaths(t *testing.T) {
	m := newMockUpstream(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { w.Write(okResponse) }))
	defer m.Close()

	t.Run("invalid passthrough options", func(t *testing.T) {
		// Duplicated /api.
		_, err := NewRoutes(m.url, proxyLabel, WithPassthroughPaths([]string{"/api1", "/api2/something", "/api1"}))
		if err == nil {
			t.Fatal("expected error")
		}
		// Wrong format, params in path.
		_, err = NewRoutes(m.url, proxyLabel, WithPassthroughPaths([]string{"/api1?args=1", "/api1"}))
		if err == nil {
			t.Fatal("expected error")
		}
		// / is not allowed.
		_, err = NewRoutes(m.url, proxyLabel, WithPassthroughPaths([]string{"/", "/api2/something", "/api1"}))
		if err == nil {
			t.Fatal("expected error")
		}
		// "" is not allowed.
		_, err = NewRoutes(m.url, proxyLabel, WithPassthroughPaths([]string{"/api1", "/api2/something", "", "/api3"}))
		if err == nil {
			t.Fatal("expected error")
		}
		// Duplication with existing enforced path is not allowed.
		_, err = NewRoutes(m.url, proxyLabel, WithPassthroughPaths([]string{"/api1", "/api2/something", "/federate", "/api3"}))
		if err == nil {
			t.Fatal("expected error")
		}
		// Duplication with existing enforced path is not allowed.
		_, err = NewRoutes(m.url, proxyLabel, WithPassthroughPaths([]string{"/api1", "/api2/something", "/federate/", "/api3"}))
		if err == nil {
			t.Fatal("expected error")
		}
		// Duplication with existing enforced path is not allowed.
		_, err = NewRoutes(m.url, proxyLabel, WithPassthroughPaths([]string{"/api1", "/api2/something", "/federate/some", "/api3"}))
		if err == nil {
			t.Fatal("expected error")
		}
		// api4 is not valid URL path (does not start with /)
		_, err = NewRoutes(m.url, proxyLabel, WithPassthroughPaths([]string{"/api1", "/api2/something", "api4", "/api3"}))
		if err == nil {
			t.Fatal("expected error")
		}
		// api4/ is not valid URL path (does not start with /)
		_, err = NewRoutes(m.url, proxyLabel, WithPassthroughPaths([]string{"/api1", "/api2/something", "api4/", "/api3"}))
		if err == nil {
			t.Fatal("expected error")
		}
		// api4/something is not valid URL path (does not start with /)
		_, err = NewRoutes(m.url, proxyLabel, WithPassthroughPaths([]string{"/api1", "/api2/something", "api4/something", "/api3"}))
		if err == nil {
			t.Fatal("expected error")
		}
	})
	r, err := NewRoutes(m.url, proxyLabel, WithPassthroughPaths([]string{"/api1", "/api2/something", "/graph/"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, tcase := range []struct {
		url     string
		method  string
		expCode int
	}{
		{
			url: "http://prometheus.example.com/graph?namespace=ns1", method: http.MethodGet,
			expCode: http.StatusOK,
		},
		{
			url: "http://prometheus.example.com/graph", method: http.MethodPost,
			expCode: http.StatusOK,
		},
		{
			url: "http://prometheus.example.com/graph2", method: http.MethodPost,
			expCode: http.StatusNotFound,
		},
		{
			url: "http://prometheus.example.com/api/v2/silence", method: http.MethodGet,
			expCode: http.StatusBadRequest, // Missing label to inject.
		},
		{
			url: "http://prometheus.example.com/api1?yolo=ns1", method: http.MethodGet,
			expCode: http.StatusOK,
		},
		{
			url: "http://prometheus.example.com/api2/something", method: http.MethodGet,
			expCode: http.StatusOK,
		},
	} {
		t.Run(tcase.url, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(tcase.method, tcase.url, nil))
			resp := w.Result()
			if resp.StatusCode != tcase.expCode {
				b, err := ioutil.ReadAll(resp.Body)
				fmt.Println(string(b), err)
				t.Fatalf("expected status code %v, got %d", tcase.expCode, resp.StatusCode)
			}
		})
	}
}

func TestMatch(t *testing.T) {
	for _, tc := range []struct {
		labelv  string
		matches []string

		expCode  int
		expMatch []string
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
			expMatch: []string{`{namespace="default"}`},
			expBody:  okResponse,
		},
		{
			// Single "match" parameters.
			labelv:   "default",
			matches:  []string{`{job="prometheus",__name__=~"job:.*"}`},
			expCode:  http.StatusOK,
			expMatch: []string{`{job="prometheus",__name__=~"job:.*",namespace="default"}`},
			expBody:  okResponse,
		},
		{
			// Single "match" parameters with label dup name.
			labelv:   "default",
			matches:  []string{`{job="prometheus",__name__=~"job:.*",namespace="default"}`},
			expCode:  http.StatusOK,
			expMatch: []string{`{job="prometheus",__name__=~"job:.*",namespace="default",namespace="default"}`},
			expBody:  okResponse,
		},
		{
			// Many "match" parameters.
			labelv:   "default",
			matches:  []string{`{job="prometheus"}`, `{__name__=~"job:.*"}`},
			expCode:  http.StatusOK,
			expMatch: []string{`{job="prometheus",namespace="default"}`, `{__name__=~"job:.*",namespace="default"}`},
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
						checkQueryHandler("", matchersParam, tc.expMatch...),
					),
				)
				defer m.Close()
				r, err := NewRoutes(m.url, proxyLabel, WithEnabledLabelsAPI())
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				u, err := url.Parse(u)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				q := u.Query()
				for _, m := range tc.matches {
					q.Add(matchersParam, m)
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

func TestMatchWithPost(t *testing.T) {
	for _, tc := range []struct {
		labelv  string
		matches []string

		expCode  int
		expMatch []string
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
			expMatch: []string{`{namespace="default"}`},
			expBody:  okResponse,
		},
		{
			// Single "match" parameters.
			labelv:   "default",
			matches:  []string{`{job="prometheus",__name__=~"job:.*"}`},
			expCode:  http.StatusOK,
			expMatch: []string{`{job="prometheus",__name__=~"job:.*",namespace="default"}`},
			expBody:  okResponse,
		},
		{
			// Single "match" parameters with label dup name.
			labelv:   "default",
			matches:  []string{`{job="prometheus",__name__=~"job:.*",namespace="default"}`},
			expCode:  http.StatusOK,
			expMatch: []string{`{job="prometheus",__name__=~"job:.*",namespace="default",namespace="default"}`},
			expBody:  okResponse,
		},
		{
			// Many "match" parameters.
			labelv:   "default",
			matches:  []string{`{job="prometheus"}`, `{__name__=~"job:.*"}`},
			expCode:  http.StatusOK,
			expMatch: []string{`{job="prometheus",namespace="default"}`, `{__name__=~"job:.*",namespace="default"}`},
			expBody:  okResponse,
		},
	} {
		for _, u := range []string{
			"http://prometheus.example.com/api/v1/labels",
		} {
			t.Run(fmt.Sprintf("%s?match[]=%s", u, strings.Join(tc.matches, "&")), func(t *testing.T) {
				m := newMockUpstream(
					checkFormParameterAbsent(
						proxyLabel,
						checkFormHandler(matchersParam, tc.expMatch...),
					),
				)
				defer m.Close()
				r, err := NewRoutes(m.url, proxyLabel, WithEnabledLabelsAPI())
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				u, err := url.Parse(u)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				q := url.Values{}
				for _, m := range tc.matches {
					q.Add(matchersParam, m)
				}
				q.Set(proxyLabel, tc.labelv)

				w := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodPost, u.String(), strings.NewReader(q.Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
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

func TestSeries(t *testing.T) {
	for _, tc := range []struct {
		name        string
		labelv      string
		promQuery   string
		expResponse []byte
		expCode     int
		expMatch    []string
		expBody     []byte
	}{
		{
			name:    `No "namespace" parameter returns an error`,
			expCode: http.StatusBadRequest,
		},
		{
			name:    `No "namespace" parameter returns an error for POSTs`,
			expCode: http.StatusBadRequest,
		},
		{
			name:        `No "match[]" parameter returns 200 with empty body`,
			labelv:      "default",
			expMatch:    []string{`{namespace="default"}`},
			expResponse: okResponse,
			expCode:     http.StatusOK,
		},
		{
			name:        `No "match[]" parameter returns 200 with empty body for POSTs`,
			labelv:      "default",
			expMatch:    []string{`{namespace="default"}`},
			expResponse: okResponse,
			expCode:     http.StatusOK,
		},
		{
			name:        `Series`,
			labelv:      "default",
			promQuery:   "up",
			expCode:     http.StatusOK,
			expMatch:    []string{`{__name__="up",namespace="default"}`},
			expResponse: okResponse,
		},
		{
			name:        `Series with labels`,
			labelv:      "default",
			promQuery:   `up{instance="localhost:9090"}`,
			expCode:     http.StatusOK,
			expMatch:    []string{`{instance="localhost:9090",__name__="up",namespace="default"}`},
			expResponse: okResponse,
		},
	} {
		for _, endpoint := range []string{"series"} {
			t.Run(endpoint+"/"+strings.ReplaceAll(tc.name, " ", "_"), func(t *testing.T) {
				m := newMockUpstream(
					checkParameterAbsent(
						proxyLabel,
						checkQueryHandler("", matchersParam, tc.expMatch...),
					),
				)
				defer m.Close()
				var opts []Option
				r, err := NewRoutes(m.url, proxyLabel, opts...)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				u, err := url.Parse("http://prometheus.example.com/api/v1/" + endpoint)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				q := u.Query()
				if tc.promQuery != "" {
					q.Add(matchersParam, tc.promQuery)
				}
				q.Set(proxyLabel, tc.labelv)
				u.RawQuery = q.Encode()

				w := httptest.NewRecorder()
				req := httptest.NewRequest("GET", u.String(), nil)
				r.ServeHTTP(w, req)

				resp := w.Result()

				body, err := ioutil.ReadAll(resp.Body)

				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				defer resp.Body.Close()

				if resp.StatusCode != tc.expCode {
					t.Logf("expected status code %d, got %d", tc.expCode, resp.StatusCode)
					t.Logf("%s", string(body))
					t.FailNow()
				}
				if resp.StatusCode != http.StatusOK {
					return
				}
				if string(body) != string(tc.expResponse) {
					t.Fatalf("expected response body %q, got %q", string(tc.expResponse), string(body))
				}
			})
		}
	}
}

func TestSeriesWithPost(t *testing.T) {
	for _, tc := range []struct {
		name          string
		labelv        string
		promQueryBody string
		expResponse   []byte
		method        string
		expCode       int
		expMatch      []string
		expBody       []byte
	}{
		{
			name:    `No "namespace" parameter returns an error`,
			expCode: http.StatusBadRequest,
		},
		{
			name:    `No "namespace" parameter returns an error for POSTs`,
			expCode: http.StatusBadRequest,
			method:  http.MethodPost,
		},
		{
			name:        `No "match[]" parameter returns 200 with empty body`,
			labelv:      "default",
			method:      http.MethodPost,
			expMatch:    []string{`{namespace="default"}`},
			expResponse: okResponse,
			expCode:     http.StatusOK,
		},
		{
			name:        `No "match[]" parameter returns 200 with empty body for POSTs`,
			method:      http.MethodPost,
			labelv:      "default",
			expMatch:    []string{`{namespace="default"}`},
			expResponse: okResponse,
			expCode:     http.StatusOK,
		},
		{
			name:          `Series POST`,
			labelv:        "default",
			promQueryBody: "up",
			method:        http.MethodPost,
			expCode:       http.StatusOK,
			expMatch:      []string{`{__name__="up",namespace="default"}`},
			expResponse:   okResponse,
		},
		{
			name:          `Series with labels POST`,
			labelv:        "default",
			promQueryBody: `up{instance="localhost:9090"}`,
			method:        http.MethodPost,
			expCode:       http.StatusOK,
			expMatch:      []string{`{instance="localhost:9090",__name__="up",namespace="default"}`},
			expResponse:   okResponse,
		},
	} {
		for _, endpoint := range []string{"series"} {
			t.Run(endpoint+"/"+strings.ReplaceAll(tc.name, " ", "_"), func(t *testing.T) {
				m := newMockUpstream(
					checkParameterAbsent(
						proxyLabel,
						checkFormHandler(matchersParam, tc.expMatch...),
					),
				)
				defer m.Close()
				var opts []Option
				r, err := NewRoutes(m.url, proxyLabel, opts...)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				u, err := url.Parse("http://prometheus.example.com/api/v1/" + endpoint)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				q := u.Query()
				q.Set(proxyLabel, tc.labelv)
				u.RawQuery = q.Encode()

				var b io.Reader = nil
				if tc.promQueryBody != "" {
					b = strings.NewReader(url.Values(map[string][]string{"match[]": {tc.promQueryBody}}).Encode())
				}
				w := httptest.NewRecorder()
				req := httptest.NewRequest(tc.method, u.String(), b)
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				r.ServeHTTP(w, req)

				resp := w.Result()

				body, err := ioutil.ReadAll(resp.Body)

				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				defer resp.Body.Close()

				if resp.StatusCode != tc.expCode {
					t.Logf("expected status code %d, got %d", tc.expCode, resp.StatusCode)
					t.Logf("%s", string(body))
					t.FailNow()
				}
				if resp.StatusCode != http.StatusOK {
					return
				}
				if string(body) != string(tc.expResponse) {
					t.Fatalf("expected response body %q, got %q", string(tc.expResponse), string(body))
				}
			})
		}
	}
}

func TestQuery(t *testing.T) {
	for _, tc := range []struct {
		name          string
		labelv        string
		promQuery     string
		promQueryBody string
		method        string

		expCode          int
		expPromQuery     string
		expPromQueryBody string
		expResponse      []byte
		errorOnReplace   bool
	}{
		{
			name:    `No "namespace" parameter returns an error`,
			expCode: http.StatusBadRequest,
		},
		{
			name:    `No "namespace" parameter returns an error for POSTs`,
			expCode: http.StatusBadRequest,
			method:  http.MethodPost,
		},
		{
			name:    `No "query" parameter returns 200 with empty body`,
			labelv:  "default",
			expCode: http.StatusOK,
		},
		{
			name:    `No "query" parameter returns 200 with empty body for POSTs`,
			labelv:  "default",
			expCode: http.StatusOK,
		},
		{
			name:         `Query without a vector selector`,
			labelv:       "default",
			promQuery:    "up",
			expCode:      http.StatusOK,
			expPromQuery: `up{namespace="default"}`,
			expResponse:  okResponse,
		},
		{
			name:             `Query without a vector selector in POST body`,
			labelv:           "default",
			promQueryBody:    "up",
			method:           http.MethodPost,
			expCode:          http.StatusOK,
			expPromQueryBody: `up{namespace="default"}`,
			expResponse:      okResponse,
		},
		{
			name:             `Tricky: Query without a vector selector in GET body (yes, that's possible)'`,
			labelv:           "default",
			promQueryBody:    "up",
			method:           http.MethodGet,
			expCode:          http.StatusOK,
			expPromQueryBody: ``, // We should finish request without forwarding. Form should not parse this value for GET.
		},
		{
			name:             `Query without a vector selector in POST body or query`,
			labelv:           "default",
			promQuery:        "up",
			promQueryBody:    "up",
			method:           http.MethodPost,
			expCode:          http.StatusOK,
			expPromQuery:     `up{namespace="default"}`,
			expPromQueryBody: `up{namespace="default"}`,
			expResponse:      okResponse,
		},
		{
			name:             `Query without a vector selector in POST body or query different`,
			labelv:           "default",
			promQuery:        "up",
			promQueryBody:    "foo",
			method:           http.MethodPost,
			expCode:          http.StatusOK,
			expPromQuery:     `up{namespace="default"}`,
			expPromQueryBody: `foo{namespace="default"}`,
			expResponse:      okResponse,
		},
		{
			name:         `Query with a vector selector`,
			labelv:       "default",
			promQuery:    `up{namespace="other"}`,
			expCode:      http.StatusOK,
			expPromQuery: `up{namespace="default"}`,
			expResponse:  okResponse,
		},
		{
			name:             `Query with a vector selector in POST body`,
			labelv:           "default",
			promQueryBody:    `up{namespace="other"}`,
			method:           http.MethodPost,
			expCode:          http.StatusOK,
			expPromQueryBody: `up{namespace="default"}`,
			expResponse:      okResponse,
		},
		{
			name:           `Query with a vector selector and errorOnReplace`,
			labelv:         "default",
			promQuery:      `up{namespace="other"}`,
			errorOnReplace: true,
			expCode:        http.StatusBadRequest,
			expResponse:    nil,
		},
		{
			name:           `Query with a vector selector in POST body and errorOnReplace`,
			labelv:         "default",
			promQueryBody:  `up{namespace="other"}`,
			method:         http.MethodPost,
			errorOnReplace: true,
			expCode:        http.StatusBadRequest,
			expResponse:    nil,
		},
		{
			name:         `Query with a scalar`,
			labelv:       "default",
			promQuery:    "1",
			expCode:      http.StatusOK,
			expPromQuery: `1`,
			expResponse:  okResponse,
		},
		{
			name:             `Query with a scalar in POST body`,
			labelv:           "default",
			promQueryBody:    "1",
			method:           http.MethodPost,
			expCode:          http.StatusOK,
			expPromQueryBody: `1`,
			expResponse:      okResponse,
		},
		{
			name:         `Query with a function`,
			labelv:       "default",
			promQuery:    "time()",
			expCode:      http.StatusOK,
			expPromQuery: `time()`,
			expResponse:  okResponse,
		},
		{
			name:             `Query with a function in POST body`,
			labelv:           "default",
			promQueryBody:    "time()",
			method:           http.MethodPost,
			expCode:          http.StatusOK,
			expPromQueryBody: `time()`,
			expResponse:      okResponse,
		},
		{
			name:      `An invalid expression returns 400 with error response`,
			labelv:    "default",
			promQuery: "up +",
			expCode:   http.StatusBadRequest,
		},
		{
			name:          `An invalid expression in POST body returns 400 with error response`,
			labelv:        "default",
			promQueryBody: "up +",
			method:        http.MethodPost,
			expCode:       http.StatusBadRequest,
		},
		{
			name:         `Binary expression`,
			labelv:       "default",
			promQuery:    `up{instance="localhost:9090"} + foo{namespace="other"}`,
			expCode:      http.StatusOK,
			expPromQuery: `up{instance="localhost:9090",namespace="default"} + foo{namespace="default"}`,
			expResponse:  okResponse,
		},
	} {
		for _, endpoint := range []string{"query", "query_range", "query_exemplars"} {
			t.Run(endpoint+"/"+strings.ReplaceAll(tc.name, " ", "_"), func(t *testing.T) {
				var expBody string
				if tc.expPromQueryBody != "" {
					expBody = url.Values(map[string][]string{"query": {tc.expPromQueryBody}}).Encode()
				}
				m := newMockUpstream(
					checkParameterAbsent(
						proxyLabel,
						checkQueryHandler(expBody, queryParam, tc.expPromQuery),
					),
				)
				defer m.Close()
				var opts []Option
				if tc.errorOnReplace {
					opts = append(opts, WithErrorOnReplace())
				}
				r, err := NewRoutes(m.url, proxyLabel, opts...)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				u, err := url.Parse("http://prometheus.example.com/api/v1/" + endpoint)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				q := u.Query()
				q.Set(queryParam, tc.promQuery)
				q.Set(proxyLabel, tc.labelv)
				u.RawQuery = q.Encode()

				var b io.Reader = nil
				if tc.promQueryBody != "" {
					b = strings.NewReader(url.Values(map[string][]string{"query": {tc.promQueryBody}}).Encode())
				}
				w := httptest.NewRecorder()
				req := httptest.NewRequest(tc.method, u.String(), b)
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				r.ServeHTTP(w, req)

				resp := w.Result()
				body, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				defer resp.Body.Close()

				if resp.StatusCode != tc.expCode {
					t.Logf("expected status code %d, got %d", tc.expCode, resp.StatusCode)
					t.Logf("%s", string(body))
					t.FailNow()
				}
				if resp.StatusCode != http.StatusOK {
					return
				}
				if string(body) != string(tc.expResponse) {
					t.Fatalf("expected response body %q, got %q", string(tc.expResponse), string(body))
				}
			})
		}
	}
}
