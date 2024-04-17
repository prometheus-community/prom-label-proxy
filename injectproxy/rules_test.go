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
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"gotest.tools/v3/golden"
)

type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

func gzipHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gz := gzip.NewWriter(w)
		defer gz.Close()

		w.Header().Del("Content-Length")
		w.Header().Set("Content-Encoding", "gzip")
		next.ServeHTTP(&gzipResponseWriter{Writer: gz, ResponseWriter: w}, r)
	})
}

func validRules() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
  "status": "success",
  "data": {
    "groups": [
      {
        "name": "group1",
        "file": "testdata/rules1.yml",
        "rules": [
          {
            "name": "metric1",
            "query": "0",
            "labels": {
              "namespace": "ns1"
            },
            "health": "ok",
            "type": "recording",
            "evaluationTime": 0.000214303,
            "lastEvaluation": "2024-04-29T14:23:52.403557247+02:00"
          },
          {
            "name": "metric2",
            "query": "1",
            "labels": {
              "namespace": "ns1",
              "operation": "create"
            },
            "health": "ok",
            "type": "recording",
            "evaluationTime": 0.000214,
            "lastEvaluation": "2024-04-29T14:23:53.403557247+02:00"
          },
          {
            "name": "metric2",
            "query": "0",
            "labels": {
              "namespace": "ns1",
              "operation": "update"
            },
            "health": "ok",
            "type": "recording",
            "evaluationTime": 0.000214,
            "lastEvaluation": "2024-04-29T14:23:54.403557247+02:00"
          },
          {
            "name": "metric2",
            "query": "0",
            "labels": {
              "namespace": "ns1",
              "operation": "delete"
            },
            "health": "ok",
            "type": "recording",
            "evaluationTime": 0.000214,
            "lastEvaluation": "2024-04-29T14:23:53.603557247+02:00"
          },
          {
            "state": "firing",
            "name": "Alert1",
            "query": "metric1{namespace=\"ns1\"} == 0",
            "duration": 0,
            "labels": {
              "namespace": "ns1"
            },
            "annotations": {},
            "alerts": [
              {
                "labels": {
                  "alertname": "Alert1",
                  "namespace": "ns1"
                },
                "annotations": {},
                "state": "firing",
                "activeAt": "2019-12-18T13:14:44.543981127+01:00",
                "value": "0e+00"
              }
            ],
            "health": "ok",
            "type": "alerting",
            "evaluationTime": 0.000214,
            "lastEvaluation": "2024-04-29T14:23:53.803557247+02:00"
          },
          {
            "state": "firing",
            "name": "Alert2",
            "query": "metric2{namespace=\"ns1\"} == 0",
            "duration": 0,
            "labels": {
              "namespace": "ns1"
            },
            "annotations": {},
            "alerts": [
              {
                "labels": {
                  "alertname": "Alert2",
                  "namespace": "ns1",
                  "operation": "update"
                },
                "annotations": {},
                "state": "firing",
                "activeAt": "2019-12-18T13:14:44.543981127+01:00",
                "value": "0e+00"
              },
              {
                "labels": {
                  "alertname": "Alert2",
                  "namespace": "ns1",
                  "operation": "delete"
                },
                "annotations": {},
                "state": "firing",
                "activeAt": "2019-12-18T13:14:44.543981127+01:00",
                "value": "0e+00"
              }
            ],
            "health": "ok",
            "type": "alerting",
            "evaluationTime": 0.000214,
            "lastEvaluation": "2024-04-29T14:23:53.903557247+02:00"
          }
        ],
        "interval": 10
      },
      {
        "name": "group1",
        "file": "testdata/rules2.yml",
        "rules": [
          {
            "name": "metric1",
            "query": "1",
            "labels": {
              "namespace": "ns2"
            },
            "health": "ok",
            "type": "recording",
            "evaluationTime": 0.000214303,
            "lastEvaluation": "2024-04-29T14:23:52.403557247+02:00"
          },
          {
            "state": "inactive",
            "name": "Alert1",
            "query": "metric1{namespace=\"ns2\"} == 0",
            "duration": 0,
            "labels": {
              "namespace": "ns2"
            },
            "annotations": {},
            "alerts": [],
            "health": "ok",
            "type": "alerting",
            "evaluationTime": 0.000214,
            "lastEvaluation": "2024-04-29T14:23:52.503557247+02:00"
          }
        ],
        "interval": 10
      },
      {
        "name": "group2",
        "file": "testdata/rules2.yml",
        "rules": [
          {
            "name": "metric2",
            "query": "1",
            "labels": {
              "namespace": "ns2",
              "operation": "create"
            },
            "health": "ok",
            "type": "recording",
            "evaluationTime": 0.000214,
            "lastEvaluation": "2024-04-29T14:23:52.503557247+02:00"
          },
          {
            "name": "metric2",
            "query": "2",
            "labels": {
              "namespace": "ns2",
              "operation": "update"
            },
            "health": "ok",
            "type": "recording",
            "evaluationTime": 0.000214,
            "lastEvaluation": "2024-04-29T14:23:52.603557247+02:00"
          },
          {
            "name": "metric2",
            "query": "3",
            "labels": {
              "namespace": "ns2",
              "operation": "delete"
            },
            "health": "ok",
            "type": "recording",
            "evaluationTime": 0.000214,
            "lastEvaluation": "2024-04-29T14:23:52.643557247+02:00"
          },
          {
            "name": "metric3",
            "query": "0",
            "labels": {
              "namespace": "ns2"
            },
            "health": "ok",
            "type": "recording",
            "evaluationTime": 0.000214,
            "lastEvaluation": "2024-04-29T14:23:52.683557247+02:00"
          },
          {
            "state": "inactive",
            "name": "Alert2",
            "query": "metric2{namespace=\"ns2\"} == 0",
            "duration": 0,
            "labels": {
              "namespace": "ns2"
            },
            "annotations": {},
            "alerts": [],
            "health": "ok",
            "type": "alerting",
            "evaluationTime": 0.000214,
            "lastEvaluation": "2024-04-29T14:23:52.803557247+02:00"
          },
          {
            "state": "firing",
            "name": "Alert3",
            "query": "metric3{namespace=\"ns2\"} == 0",
            "duration": 0,
            "labels": {
              "namespace": "ns2"
            },
            "annotations": {},
            "alerts": [
              {
                "labels": {
                  "alertname": "Alert3",
                  "namespace": "ns2"
                },
                "annotations": {},
                "state": "firing",
                "activeAt": "2019-12-18T13:14:39.972915521+01:00",
                "value": "0e+00"
              }
            ],
            "health": "ok",
            "type": "alerting",
            "evaluationTime": 0.000214,
            "lastEvaluation": "2024-04-29T14:23:52.903557247+02:00"
          }
        ],
        "interval": 10
      }
    ]
  }
}`))
	})
}

func validAlerts() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
  "status": "success",
  "data": {
    "alerts": [
      {
        "labels": {
          "alertname": "Alert1",
          "namespace": "ns1"
        },
        "annotations": {},
        "state": "firing",
        "activeAt": "2019-12-18T13:14:44.543981127+01:00",
        "value": "0e+00"
      },
      {
        "labels": {
          "alertname": "Alert2",
          "namespace": "ns1",
          "operation": "update"
        },
        "annotations": {},
        "state": "firing",
        "activeAt": "2019-12-18T13:14:44.543981127+01:00",
        "value": "0e+00"
      },
      {
        "labels": {
          "alertname": "Alert2",
          "namespace": "ns1",
          "operation": "delete"
        },
        "annotations": {},
        "state": "firing",
        "activeAt": "2019-12-18T13:14:44.543981127+01:00",
        "value": "0e+00"
      },
      {
        "labels": {
          "alertname": "Alert3",
          "namespace": "ns2"
        },
        "annotations": {},
        "state": "firing",
        "activeAt": "2019-12-18T13:14:39.972915521+01:00",
        "value": "0e+00"
      }
    ]
  }
}`))
	})
}

func TestRules(t *testing.T) {
	for _, tc := range []struct {
		labelv     []string
		upstream   http.Handler
		reqHeaders http.Header

		expCode int
		golden  string
	}{
		{
			// No "namespace" parameter returns an error.
			expCode: http.StatusBadRequest,
			golden:  "rules_no_namespace_error.golden",
		},
		{
			// non 200 status code from upstream is passed as-is.
			labelv: []string{"upstream_error"},
			upstream: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("error"))
			}),

			expCode: http.StatusBadRequest,
			golden:  "rules_upstream_error.golden",
		},
		{
			// incomplete API response triggers a 502 error.
			labelv: []string{"incomplete_data_from_upstream"},
			upstream: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Write([]byte("{"))
			}),

			expCode: http.StatusBadGateway,
			golden:  "rules_incomplete_upstream_response.golden",
		},
		{
			// invalid API response triggers a 502 error.
			labelv: []string{"invalid_data_from_upstream"},
			upstream: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Write([]byte("0"))
			}),

			expCode: http.StatusBadGateway,
			golden:  "rules_invalid_upstream_response.golden",
		},
		{
			// "namespace" parameter matching no rule.
			labelv:   []string{"not_present"},
			upstream: validRules(),

			expCode: http.StatusOK,
			golden:  "rules_no_match.golden",
		},
		{
			// Gzipped response should be handled when explictly asked by the original client.
			labelv:   []string{"not_present_gzip_requested"},
			upstream: gzipHandler(validRules()),
			reqHeaders: map[string][]string{
				"Accept-Encoding": {"gzip"},
			},

			expCode: http.StatusOK,
			golden:  "rules_no_match_with_gzip_requested.golden",
		},
		{
			// When the client doesn't ask explicitly for gzip encoding, the Go
			// standard library will automatically ask for it and it will
			// transparently decompress the gzipped response.
			labelv:   []string{"not_present_gzip_not_requested"},
			upstream: gzipHandler(validRules()),

			expCode: http.StatusOK,
			golden:  "rules_no_match_with_gzip_not_requested.golden",
		},
		{
			labelv:   []string{"ns1"},
			upstream: validRules(),

			expCode: http.StatusOK,
			golden:  "rules_match_namespace_ns1.golden",
		},
		{
			labelv:   []string{"ns2"},
			upstream: validRules(),

			expCode: http.StatusOK,
			golden:  "rules_match_namespace_ns2.golden",
		},
		{
			labelv:   []string{"ns1", "ns2"},
			upstream: validRules(),

			expCode: http.StatusOK,
			golden:  "rules_match_namespaces_ns1_and_ns2.golden",
		},
	} {
		t.Run(fmt.Sprintf("%s=%s", proxyLabel, tc.labelv), func(t *testing.T) {
			m := newMockUpstream(tc.upstream)
			defer m.Close()
			r, err := NewRoutes(m.url, proxyLabel, HTTPFormEnforcer{ParameterName: proxyLabel})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			u, err := url.Parse("http://prometheus.example.com/api/v1/rules")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			q := u.Query()
			for _, lv := range tc.labelv {
				q.Add(proxyLabel, lv)
			}

			u.RawQuery = q.Encode()

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", u.String(), nil)
			for k, v := range tc.reqHeaders {
				for i := range v {
					req.Header.Add(k, v[i])
				}
			}
			r.ServeHTTP(w, req)

			resp := w.Result()

			if resp.StatusCode != tc.expCode {
				t.Fatalf("expected status code %d, got %d", tc.expCode, resp.StatusCode)
			}

			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusOK {
				golden.Assert(t, string(body), tc.golden)
				return
			}

			// We need to unmarshal/marshal the result to run deterministic comparisons.
			got := normalizeAPIResponse(t, body)
			golden.Assert(t, got, tc.golden)
		})
	}
}

func TestAlerts(t *testing.T) {
	for _, tc := range []struct {
		labelv   []string
		upstream http.Handler

		expCode int
		golden  string
	}{
		{
			// No "namespace" parameter returns an error.
			expCode: http.StatusBadRequest,
			golden:  "alerts_no_namespace_error.golden",
		},
		{
			// non 200 status code from upstream is passed as-is.
			labelv: []string{"upstream_error"},
			upstream: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("error"))
			}),

			expCode: http.StatusBadRequest,
			golden:  "alerts_upstream_error.golden",
		},
		{
			// incomplete API response triggers a 502 error.
			labelv: []string{"incomplete_data_from_upstream"},
			upstream: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Write([]byte("{"))
			}),

			expCode: http.StatusBadGateway,
			golden:  "alerts_incomplete_upstream_response.golden",
		},
		{
			// invalid API response triggers a 502 error.
			labelv: []string{"invalid_data_from_upstream"},
			upstream: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Write([]byte("0"))
			}),

			expCode: http.StatusBadGateway,
			golden:  "alerts_invalid_upstream_response.golden",
		},
		{
			// "namespace" parameter matching no rule.
			labelv:   []string{"not_present"},
			upstream: validAlerts(),

			expCode: http.StatusOK,
			golden:  "alerts_no_match.golden",
		},
		{
			labelv:   []string{"ns1"},
			upstream: validAlerts(),

			expCode: http.StatusOK,
			golden:  "alerts_match_namespace_ns1.golden",
		},
		{
			labelv:   []string{"ns2"},
			upstream: validAlerts(),

			expCode: http.StatusOK,
			golden:  "alerts_match_namespace_ns2.golden",
		},
		{
			labelv:   []string{"ns1", "ns2"},
			upstream: validAlerts(),

			expCode: http.StatusOK,
			golden:  "alerts_match_namespaces_ns1_and_ns2.golden",
		},
	} {
		t.Run(fmt.Sprintf("%s=%s", proxyLabel, tc.labelv), func(t *testing.T) {
			m := newMockUpstream(tc.upstream)
			defer m.Close()
			r, err := NewRoutes(m.url, proxyLabel, HTTPFormEnforcer{ParameterName: proxyLabel})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			u, err := url.Parse("http://prometheus.example.com/api/v1/alerts")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			q := u.Query()
			for _, lv := range tc.labelv {
				q.Add(proxyLabel, lv)
			}
			u.RawQuery = q.Encode()

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", u.String(), nil)
			r.ServeHTTP(w, req)

			resp := w.Result()

			if resp.StatusCode != tc.expCode {
				t.Fatalf("expected status code %d, got %d", tc.expCode, resp.StatusCode)
			}

			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusOK {
				golden.Assert(t, string(body), tc.golden)
				return
			}

			// We need to unmarshal/marshal the result to run deterministic comparisons.
			got := normalizeAPIResponse(t, body)
			golden.Assert(t, got, tc.golden)
		})
	}
}

func normalizeAPIResponse(t *testing.T, b []byte) string {
	t.Helper()
	var apir apiResponse
	if err := json.Unmarshal(b, &apir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out, err := json.MarshalIndent(&apir, "", "  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	return string(out)
}
