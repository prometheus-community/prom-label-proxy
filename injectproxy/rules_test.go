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
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
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
            "type": "recording"
          },
          {
            "name": "metric2",
            "query": "1",
            "labels": {
              "namespace": "ns1",
              "operation": "create"
            },
            "health": "ok",
            "type": "recording"
          },
          {
            "name": "metric2",
            "query": "0",
            "labels": {
              "namespace": "ns1",
              "operation": "update"
            },
            "health": "ok",
            "type": "recording"
          },
          {
            "name": "metric2",
            "query": "0",
            "labels": {
              "namespace": "ns1",
              "operation": "delete"
            },
            "health": "ok",
            "type": "recording"
          },
          {
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
            "type": "alerting"
          },
          {
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
            "type": "alerting"
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
            "type": "recording"
          },
          {
            "name": "Alert1",
            "query": "metric1{namespace=\"ns2\"} == 0",
            "duration": 0,
            "labels": {
              "namespace": "ns2"
            },
            "annotations": {},
            "alerts": [],
            "health": "ok",
            "type": "alerting"
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
            "type": "recording"
          },
          {
            "name": "metric2",
            "query": "2",
            "labels": {
              "namespace": "ns2",
              "operation": "update"
            },
            "health": "ok",
            "type": "recording"
          },
          {
            "name": "metric2",
            "query": "3",
            "labels": {
              "namespace": "ns2",
              "operation": "delete"
            },
            "health": "ok",
            "type": "recording"
          },
          {
            "name": "metric3",
            "query": "0",
            "labels": {
              "namespace": "ns2"
            },
            "health": "ok",
            "type": "recording"
          },
          {
            "name": "Alert2",
            "query": "metric2{namespace=\"ns2\"} == 0",
            "duration": 0,
            "labels": {
              "namespace": "ns2"
            },
            "annotations": {},
            "alerts": [],
            "health": "ok",
            "type": "alerting"
          },
          {
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
            "type": "alerting"
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
		labelv     string
		upstream   http.Handler
		reqHeaders http.Header

		expCode int
		expBody []byte
	}{
		{
			// No "namespace" parameter returns an error.
			expCode: http.StatusBadRequest,
			expBody: []byte("Bad request. The \"namespace\" query parameter must be provided.\n"),
		},
		{
			// non 200 status code from upstream is passed as-is.
			labelv: "upstream_error",
			upstream: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("error"))
			}),

			expCode: http.StatusBadRequest,
			expBody: []byte("error"),
		},
		{
			// incomplete API response triggers a 502 error.
			labelv: "incomplete_data_from_upstream",
			upstream: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Write([]byte("{"))
			}),

			expCode: http.StatusBadGateway,
			expBody: []byte(""),
		},
		{
			// invalid API response triggers a 502 error.
			labelv: "invalid_data_from_upstream",
			upstream: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Write([]byte("0"))
			}),

			expCode: http.StatusBadGateway,
			expBody: []byte(""),
		},
		{
			// "namespace" parameter matching no rule.
			labelv:   "not_present",
			upstream: validRules(),

			expCode: http.StatusOK,
			expBody: []byte(`{
  "status": "success",
  "data": {
    "groups": []
  }
}`),
		},
		{
			// Gzipped response should be handled when explictly asked by the original client.
			labelv:   "not_present_gzip_requested",
			upstream: gzipHandler(validRules()),
			reqHeaders: map[string][]string{
				"Accept-Encoding": []string{"gzip"},
			},

			expCode: http.StatusOK,
			expBody: []byte(`{
  "status": "success",
  "data": {
    "groups": []
  }
}`),
		},
		{
			// When the client doesn't ask explicitly for gzip encoding, the Go
			// standard library will automatically ask for it and it will
			// transparently decompress the gzipped response.
			labelv:   "not_present_gzip_not_requested",
			upstream: gzipHandler(validRules()),

			expCode: http.StatusOK,
			expBody: []byte(`{
  "status": "success",
  "data": {
    "groups": []
  }
}`),
		},
		{
			labelv:   "ns1",
			upstream: validRules(),

			expCode: http.StatusOK,
			expBody: []byte(`{
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
            "type": "recording"
          },
          {
            "name": "metric2",
            "query": "1",
            "labels": {
              "namespace": "ns1",
              "operation": "create"
            },
            "health": "ok",
            "type": "recording"
          },
          {
            "name": "metric2",
            "query": "0",
            "labels": {
              "namespace": "ns1",
              "operation": "update"
            },
            "health": "ok",
            "type": "recording"
          },
          {
            "name": "metric2",
            "query": "0",
            "labels": {
              "namespace": "ns1",
              "operation": "delete"
            },
            "health": "ok",
            "type": "recording"
          },
          {
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
            "type": "alerting"
          },
          {
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
            "type": "alerting"
          }
        ],
        "interval": 10
      }
    ]
  }
}`),
		},
		{
			labelv:   "ns2",
			upstream: validRules(),

			expCode: http.StatusOK,
			expBody: []byte(`{
  "status": "success",
  "data": {
    "groups": [
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
            "type": "recording"
          },
          {
            "name": "Alert1",
            "query": "metric1{namespace=\"ns2\"} == 0",
            "duration": 0,
            "labels": {
              "namespace": "ns2"
            },
            "annotations": {},
            "alerts": [],
            "health": "ok",
            "type": "alerting"
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
            "type": "recording"
          },
          {
            "name": "metric2",
            "query": "2",
            "labels": {
              "namespace": "ns2",
              "operation": "update"
            },
            "health": "ok",
            "type": "recording"
          },
          {
            "name": "metric2",
            "query": "3",
            "labels": {
              "namespace": "ns2",
              "operation": "delete"
            },
            "health": "ok",
            "type": "recording"
          },
          {
            "name": "metric3",
            "query": "0",
            "labels": {
              "namespace": "ns2"
            },
            "health": "ok",
            "type": "recording"
          },
          {
            "name": "Alert2",
            "query": "metric2{namespace=\"ns2\"} == 0",
            "duration": 0,
            "labels": {
              "namespace": "ns2"
            },
            "annotations": {},
            "alerts": [],
            "health": "ok",
            "type": "alerting"
          },
          {
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
            "type": "alerting"
          }
        ],
        "interval": 10
      }
    ]
  }
}`),
		},
	} {
		t.Run(fmt.Sprintf("%s=%s", proxyLabel, tc.labelv), func(t *testing.T) {
			m := newMockUpstream(tc.upstream)
			defer m.Close()
			r, err := NewRoutes(m.url, proxyLabel)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			u, err := url.Parse("http://prometheus.example.com/api/v1/rules")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			q := u.Query()
			q.Set(proxyLabel, tc.labelv)
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

			body, _ := ioutil.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusOK {
				if string(body) != string(tc.expBody) {
					t.Fatalf("expected: %q, got: %q", string(tc.expBody), string(body))
				}
				return
			}

			// We need to unmarshal/marshal the result to run deterministic comparisons.
			got := normalizeAPIResponse(t, body)
			expected := normalizeAPIResponse(t, tc.expBody)
			if got != expected {
				t.Logf("expected:")
				t.Logf(expected)
				t.Logf("got:")
				t.Logf(got)
				t.FailNow()
			}
		})
	}
}

func TestAlerts(t *testing.T) {
	for _, tc := range []struct {
		labelv   string
		upstream http.Handler

		expCode int
		expBody []byte
	}{
		{
			// No "namespace" parameter returns an error.
			expCode: http.StatusBadRequest,
			expBody: []byte("Bad request. The \"namespace\" query parameter must be provided.\n"),
		},
		{
			// non 200 status code from upstream is passed as-is.
			labelv: "upstream_error",
			upstream: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("error"))
			}),

			expCode: http.StatusBadRequest,
			expBody: []byte("error"),
		},
		{
			// incomplete API response triggers a 502 error.
			labelv: "incomplete_data_from_upstream",
			upstream: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Write([]byte("{"))
			}),

			expCode: http.StatusBadGateway,
			expBody: []byte(""),
		},
		{
			// invalid API response triggers a 502 error.
			labelv: "invalid_data_from_upstream",
			upstream: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Write([]byte("0"))
			}),

			expCode: http.StatusBadGateway,
			expBody: []byte(""),
		},
		{
			// "namespace" parameter matching no rule.
			labelv:   "not_present",
			upstream: validAlerts(),

			expCode: http.StatusOK,
			expBody: []byte(`{
  "status": "success",
  "data": {
    "alerts": []
  }
}`),
		},
		{
			labelv:   "ns1",
			upstream: validAlerts(),

			expCode: http.StatusOK,
			expBody: []byte(`{
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
      }
    ]
  }
}`),
		},
		{
			labelv:   "ns2",
			upstream: validAlerts(),

			expCode: http.StatusOK,
			expBody: []byte(`{
  "status": "success",
  "data": {
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
    ]
  }
}`),
		},
	} {
		t.Run(fmt.Sprintf("%s=%s", proxyLabel, tc.labelv), func(t *testing.T) {
			m := newMockUpstream(tc.upstream)
			defer m.Close()
			r, err := NewRoutes(m.url, proxyLabel)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			u, err := url.Parse("http://prometheus.example.com/api/v1/alerts")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			q := u.Query()
			q.Set(proxyLabel, tc.labelv)
			u.RawQuery = q.Encode()

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", u.String(), nil)
			r.ServeHTTP(w, req)

			resp := w.Result()

			if resp.StatusCode != tc.expCode {
				t.Fatalf("expected status code %d, got %d", tc.expCode, resp.StatusCode)
			}

			body, _ := ioutil.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusOK {
				if string(body) != string(tc.expBody) {
					t.Fatalf("expected: %q, got: %q", string(tc.expBody), string(body))
				}
				return
			}

			// We need to unmarshal/marshal the result to run deterministic comparisons.
			got := normalizeAPIResponse(t, body)
			expected := normalizeAPIResponse(t, tc.expBody)
			if got != expected {
				t.Logf("expected:")
				t.Logf(expected)
				t.Logf("got:")
				t.Logf(got)
				t.FailNow()
			}
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
