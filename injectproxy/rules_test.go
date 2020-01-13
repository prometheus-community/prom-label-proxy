package injectproxy

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func validRules() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
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
			r := NewRoutes(m.url, proxyLabel)

			u, err := url.Parse("http://prometheus.example.com/api/v1/rules")
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
			r := NewRoutes(m.url, proxyLabel)

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
