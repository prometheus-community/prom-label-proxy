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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/prometheus/alertmanager/api/v2/models"
)

func validSilences() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[
	{
		"id": "44cc7dd1-6976-44ea-8db4-8fd53a231ab2",
		"status": {
			"state": "active"
		},
		"updatedAt": "2022-07-22T09:50:04.545Z",
		"comment": "a",
		"createdBy": "a",
		"endsAt": "2022-07-22T11:49:40.163Z",
		"matchers": [
			{
				"isEqual": true,
				"isRegex": false,
				"name": "namespace",
				"value": "ns1"
			}
		],
		"startsAt": "2022-07-22T09:50:04.545Z"
	},
	{
		"id": "742b1215-1140-47a6-9fc7-5a98a6b0f99b",
		"status": {
			"state": "active"
		},
		"updatedAt": "2022-07-22T09:47:17.007Z",
		"comment": "foo",
		"createdBy": "bar",
		"endsAt": "2023-02-13T13:00:02.084Z",
		"matchers": [
			{
				"isEqual": true,
				"isRegex": true,
				"name": "namespace",
				"value": "ns1|ns2|ns3"
			},
			{
				"isEqual": true,
				"isRegex": false,
				"name": "foo",
				"value": "bar"
			}
		],
		"startsAt": "2022-07-22T09:47:17.007Z"
	},
	{
		"id": "8b454fdc-6538-423e-b988-7f64655232c8",
		"status": {
			"state": "active"
		},
		"updatedAt": "2022-07-22T09:49:10.191Z",
		"comment": "foo",
		"createdBy": "bar",
		"endsAt": "2023-02-13T13:00:02.084Z",
		"matchers": [
			{
				"isEqual": true,
				"isRegex": true,
				"name": "namespace",
				"value": "ns1|ns2"
			},
			{
				"isEqual": true,
				"isRegex": false,
				"name": "foo",
				"value": "bar"
			}
		],
		"startsAt": "2022-07-22T09:49:10.191Z"
	},
	{
		"id": "23b6a056-b53e-4a3d-a5df-6e890b59f3c4",
		"status": {
			"state": "active"
		},
		"updatedAt": "2022-07-22T09:49:20.973Z",
		"comment": "foo",
		"createdBy": "bar",
		"endsAt": "2023-02-13T13:00:02.084Z",
		"matchers": [
			{
				"isEqual": true,
				"isRegex": true,
				"name": "namespace",
				"value": "ns1"
			},
			{
				"isEqual": true,
				"isRegex": false,
				"name": "foo",
				"value": "bar"
			}
		],
		"startsAt": "2022-07-22T09:49:20.973Z"
	},
	{
		"id": "30995bd4-2fc3-46c8-bb7f-edcb7113374c",
		"status": {
			"state": "expired"
		},
		"updatedAt": "2022-07-22T09:48:00.091Z",
		"comment": "foo",
		"createdBy": "bar",
		"endsAt": "2022-07-22T09:48:00.091Z",
		"matchers": [
			{
				"isEqual": true,
				"isRegex": true,
				"name": "namespace",
				"value": "ns1|ns2"
			},
			{
				"isEqual": true,
				"isRegex": false,
				"name": "foo",
				"value": "bar"
			}
		],
		"startsAt": "2022-07-22T09:46:05.986Z"
	}
]`))
	})
}

func TestListSilences(t *testing.T) {
	for _, tc := range []struct {
		labelv     []string
		upstream   http.Handler
		reqHeaders http.Header

		expCode int
		expBody []byte
	}{
		{
			// No "namespace" parameter returns an error.
			expCode: http.StatusBadRequest,
			expBody: []byte(`{"error":"The \"namespace\" query parameter must be provided.","errorType":"prom-label-proxy","status":"error"}` + "\n"),
		},
		{
			// non 200 status code from upstream is passed as-is.
			labelv: []string{"upstream_error"},
			upstream: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("error"))
			}),

			expCode: http.StatusBadRequest,
			expBody: []byte("error"),
		},
		{
			// incomplete API response triggers a 502 error.
			labelv: []string{"incomplete_data_from_upstream"},
			upstream: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Write([]byte("{"))
			}),

			expCode: http.StatusBadGateway,
			expBody: []byte(""),
		},
		{
			// invalid API response triggers a 502 error.
			labelv: []string{"invalid_data_from_upstream"},
			upstream: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Write([]byte("0"))
			}),

			expCode: http.StatusBadGateway,
			expBody: []byte(""),
		},
		{
			// "namespace" parameter matching no silence.
			labelv:   []string{"not_present"},
			upstream: validSilences(),

			expCode: http.StatusOK,
			expBody: []byte(`[]`),
		},
		{
			labelv:   []string{"ns1"},
			upstream: validSilences(),

			expCode: http.StatusOK,
			expBody: []byte(`[
				{
					"id": "23b6a056-b53e-4a3d-a5df-6e890b59f3c4",
					"status": {
						"state": "active"
					},
					"updatedAt": "2022-07-22T09:49:20.973Z",
					"comment": "foo",
					"createdBy": "bar",
					"endsAt": "2023-02-13T13:00:02.084Z",
					"matchers": [
						{
							"isEqual": true,
							"isRegex": true,
							"name": "namespace",
							"value": "ns1"
						},
						{
							"isEqual": true,
							"isRegex": false,
							"name": "foo",
							"value": "bar"
						}
					],
					"startsAt": "2022-07-22T09:49:20.973Z"
				}
			]`),
		},
		{
			labelv:   []string{"ns1", "ns2", "ns3"},
			upstream: validSilences(),

			expCode: http.StatusOK,
			expBody: []byte(`[
				{
					"id": "742b1215-1140-47a6-9fc7-5a98a6b0f99b",
					"status": {
						"state": "active"
					},
					"updatedAt": "2022-07-22T09:47:17.007Z",
					"comment": "foo",
					"createdBy": "bar",
					"endsAt": "2023-02-13T13:00:02.084Z",
					"matchers": [
						{
							"isEqual": true,
							"isRegex": true,
							"name": "namespace",
							"value": "ns1|ns2|ns3"
						},
						{
							"isEqual": true,
							"isRegex": false,
							"name": "foo",
							"value": "bar"
						}
					],
					"startsAt": "2022-07-22T09:47:17.007Z"
				}
			]`),
		},
	} {
		t.Run(fmt.Sprintf("%s=%s", proxyLabel, tc.labelv), func(t *testing.T) {
			m := newMockUpstream(tc.upstream)
			defer m.Close()
			r, err := NewRoutes(m.url, proxyLabel, HTTPFormEnforcer{ParameterName: proxyLabel})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			u, err := url.Parse("http://alertmanager.example.com/api/v2/silences")
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
			body, _ := io.ReadAll(resp.Body)
			defer resp.Body.Close()

			if resp.StatusCode != tc.expCode {
				t.Fatalf("expected status code %d, got %d", tc.expCode, resp.StatusCode)
			}

			if resp.StatusCode != http.StatusOK {
				if string(body) != string(tc.expBody) {
					t.Fatalf("expected: %q, got: %q", string(tc.expBody), string(body))
				}
				return
			}

			// We need to unmarshal/marshal the result to run deterministic comparisons.
			got := normalizeAlertmanagerResponse(t, body)
			expected := normalizeAlertmanagerResponse(t, tc.expBody)
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

func normalizeAlertmanagerResponse(t *testing.T, b []byte) string {
	t.Helper()
	var apir models.GettableSilences
	if err := json.Unmarshal(b, &apir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out, err := json.MarshalIndent(&apir, "", "  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	return string(out)
}

const silID = "802146e0-1f7a-42a6-ab0e-1e631479970b"

func getSilenceWithoutLabel() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "GET" {
			prometheusAPIError(w, "invalid method: "+req.Method, http.StatusInternalServerError)
			return
		}
		if req.URL.Path != "/api/v2/silence/"+silID {
			prometheusAPIError(w, "invalid path: "+req.URL.Path, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `
{
  "id": "%s",
  "status": {
    "state": "pending"
  },
  "updatedAt": "2020-01-15T09:06:23.419Z",
  "comment": "comment",
  "createdBy": "author",
  "endsAt": "2020-02-13T13:00:02.084Z",
  "matchers": [
    {
      "isRegex": false,
      "name": "foo",
      "value": "bar"
    }
  ],
  "startsAt": "2020-02-13T12:02:01.000Z"
}
				`, silID)
	})
}

func getSilenceWithLabel(labelv string, regex bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "GET" {
			prometheusAPIError(w, "invalid method: "+req.Method, http.StatusInternalServerError)
			return
		}
		if req.URL.Path != "/api/v2/silence/"+silID {
			prometheusAPIError(w, "invalid path: "+req.URL.Path, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `
{
  "id": "%s",
  "status": {
    "state": "pending"
  },
  "updatedAt": "2020-01-15T09:06:23.419Z",
  "comment": "comment",
  "createdBy": "author",
  "endsAt": "2020-02-13T13:00:02.084Z",
  "matchers": [
    {
      "isRegex": %v,
      "name": "%s",
      "value": "%s"
    }
  ],
  "startsAt": "2020-02-13T12:02:01.000Z"
}
				`, silID, regex, proxyLabel, labelv)
	})
}

func createSilenceWithLabel(labelv string, expectedMatchersCount int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var sil models.PostableSilence
		if err := json.NewDecoder(req.Body).Decode(&sil); err != nil {
			prometheusAPIError(w, fmt.Sprintf("unexpected error: %v", err), http.StatusInternalServerError)
			return
		}
		var values []string
		matcherCount := 0
		for _, m := range sil.Matchers {
			if *m.Name == proxyLabel {
				matcherCount++
				if *m.IsRegex {
					values = append(values, *m.Value)
				}
			}
		}
		if matcherCount != expectedMatchersCount {
			prometheusAPIError(w, fmt.Sprintf("expected %d matcher for label %s, got %d", expectedMatchersCount, proxyLabel, len(values)), http.StatusInternalServerError)
			return
		}
		if !contains(values, labelv) {
			prometheusAPIError(w, fmt.Sprintf("expected matcher for label %s to contain %q, got %q", proxyLabel, labelv, values), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(okResponse)
	})
}

// chainedHandlers runs the handler one after the other.
type chainedHandlers struct {
	idx      int
	handlers []http.Handler
}

func (c *chainedHandlers) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	defer func() { c.idx++ }()

	if c.idx >= len(c.handlers) {
		prometheusAPIError(w, "", http.StatusInternalServerError)
		return
	}
	c.handlers[c.idx].ServeHTTP(w, req)
}

func TestDeleteSilence(t *testing.T) {
	for _, tc := range []struct {
		ID       string
		labelv   []string
		upstream http.Handler

		expCode int
		expBody []byte
	}{
		{
			// No "namespace" parameter returns an error.
			expCode: http.StatusBadRequest,
		},
		{
			// Missing silence ID.
			ID:      "",
			labelv:  []string{"default"},
			expCode: http.StatusBadRequest,
		},
		{
			// The silence doesn't exist upstream.
			ID:     silID,
			labelv: []string{"default"},
			upstream: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				http.NotFound(w, req)
			}),
			expCode: http.StatusBadGateway,
		},
		{
			// The silence doesn't contain the expected label.
			ID:       silID,
			labelv:   []string{"default"},
			upstream: getSilenceWithoutLabel(),
			expCode:  http.StatusForbidden,
		},
		{
			// The silence doesn't have the expected value for the label.
			ID:       silID,
			labelv:   []string{"default"},
			upstream: getSilenceWithLabel("not default", false),
			expCode:  http.StatusForbidden,
		},
		{
			// The silence has the expected regex value for the label.
			ID:     silID,
			labelv: []string{"default", "something", "anything"},
			upstream: &chainedHandlers{
				handlers: []http.Handler{
					getSilenceWithLabel("anything|default|something", true),
					http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
						w.Write([]byte("ok"))
					}),
				},
			},
			expCode: http.StatusOK,
			expBody: []byte("ok"),
		},
		{
			// The silence has the expected regex value for the label.
			ID:       silID,
			labelv:   []string{"default", "something", "anything"},
			upstream: getSilenceWithLabel("default|not default", true),
			expCode:  http.StatusForbidden,
		},
		{
			// The silence has the expected value for the label but upstream returns an error.
			ID:     silID,
			labelv: []string{"default"},
			upstream: &chainedHandlers{
				handlers: []http.Handler{
					getSilenceWithLabel("default", true),
					http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
						w.WriteHeader(http.StatusTeapot)
					}),
				},
			},
			expCode: http.StatusTeapot,
		},
	} {
		t.Run("", func(t *testing.T) {
			m := newMockUpstream(tc.upstream)
			defer m.Close()
			r, err := NewRoutes(m.url, proxyLabel, HTTPFormEnforcer{ParameterName: proxyLabel})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			u, err := url.Parse(fmt.Sprintf("http://alertmanager.example.com/api/v2/silence/" + tc.ID))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			q := u.Query()

			for _, lv := range tc.labelv {
				q.Add(proxyLabel, lv)
			}

			u.RawQuery = q.Encode()

			w := httptest.NewRecorder()
			req := httptest.NewRequest("DELETE", u.String(), nil)
			r.ServeHTTP(w, req)

			resp := w.Result()
			body, _ := io.ReadAll(resp.Body)
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

func TestUpdateSilence(t *testing.T) {
	for _, tc := range []struct {
		data     string
		labelv   []string
		upstream http.Handler

		expCode int
		expBody []byte
	}{
		{
			// No "namespace" parameter returns an error.
			expCode: http.StatusBadRequest,
		},
		{
			// Invalid silence payload returns an error.
			data:    "{",
			labelv:  []string{"default"},
			expCode: http.StatusBadRequest,
		},
		{
			// Creation of a valid silence without namespace label is ok.
			data: `{
    "comment":"foo",
    "createdBy":"bar",
    "endsAt":"2020-02-13T13:00:02.084Z",
    "matchers": [
        {"isRegex":false,"Name":"foo","Value":"bar"}
    ],
    "startsAt":"2020-02-13T12:02:01Z"
}`,
			labelv:   []string{"default"},
			upstream: createSilenceWithLabel("default", 1),

			expCode: http.StatusOK,
			expBody: okResponse,
		},
		{
			// Creation of a silence with an existing namespace label is ok.
			data: `{
    "comment":"foo",
    "createdBy":"bar",
    "endsAt":"2020-02-13T13:00:02.084Z",
    "matchers": [
        {"isRegex":false,"Name":"foo","Value":"bar"},
		{"isRegex":false,"Name":"namespace","Value":"not default"}
    ],
    "startsAt":"2020-02-13T12:02:01Z"
}`,
			labelv:   []string{"default"},
			upstream: createSilenceWithLabel("default", 2),

			expCode: http.StatusOK,
			expBody: okResponse,
		},
		{
			// Creation of a silence without matcher returns an error.
			data: `{
    "comment":"foo",
    "createdBy":"bar",
    "endsAt":"2020-02-13T13:00:02.084Z",
    "matchers": [],
    "startsAt":"2020-02-13T12:02:01Z"
}`,
			labelv: []string{"default"},

			expCode: http.StatusBadRequest,
		},
		{
			// Update of an existing silence with a matching label is ok.
			data: `{
    "id":"` + silID + `",
    "comment":"foo",
    "createdBy":"bar",
    "endsAt":"2020-02-13T13:00:02.084Z",
    "matchers": [
        {"isRegex":false,"Name":"foo","Value":"bar"}
    ],
    "startsAt":"2020-02-13T12:02:01Z"
}`,
			labelv: []string{"default"},
			upstream: &chainedHandlers{
				handlers: []http.Handler{
					getSilenceWithLabel("default", true),
					createSilenceWithLabel("default", 1),
				},
			},

			expCode: http.StatusOK,
			expBody: okResponse,
		},
		{
			// Update of an existing silence with a matching label with regex matcher with multiple label values is ok.
			data: `{
    "id":"` + silID + `",
    "comment":"foo",
    "createdBy":"bar",
    "endsAt":"2020-02-13T13:00:02.084Z",
    "matchers": [
        {"isRegex":false,"Name":"foo","Value":"bar"}
    ],
    "startsAt":"2020-02-13T12:02:01Z"
}`,
			labelv: []string{"default", "something"},
			upstream: &chainedHandlers{
				handlers: []http.Handler{
					getSilenceWithLabel("default|something", true),
					createSilenceWithLabel("default|something", 1),
				},
			},

			expCode: http.StatusOK,
			expBody: okResponse,
		},
		{
			// Check that label values are correctly escaped
			data: `{
    "id":"` + silID + `",
    "comment":"foo",
    "createdBy":"bar",
    "endsAt":"2020-02-13T13:00:02.084Z",
    "matchers": [
        {"isRegex":false,"Name":"foo","Value":"bar"}
    ],
    "startsAt":"2020-02-13T12:02:01Z"
}`,
			labelv: []string{"default", "some|thing"},
			upstream: &chainedHandlers{
				handlers: []http.Handler{
					getSilenceWithLabel("default|some\\\\|thing", true),
					createSilenceWithLabel("default|some\\|thing", 1),
				},
			},

			expCode: http.StatusOK,
			expBody: okResponse,
		},
		{
			// Update of an existing silence with a non-matching label with regex matcher with multiple label values is denied.
			data: `{
    "id":"` + silID + `",
    "comment":"foo",
    "createdBy":"bar",
    "endsAt":"2020-02-13T13:00:02.084Z",
    "matchers": [
        {"isRegex":false,"Name":"foo","Value":"bar"}
    ],
    "startsAt":"2020-02-13T12:02:01Z"
}`,
			labelv: []string{"default", "something"},
			upstream: &chainedHandlers{
				handlers: []http.Handler{
					getSilenceWithLabel("default|not default", true),
					createSilenceWithLabel("default|something", 1),
				},
			},

			expCode: http.StatusForbidden,
		},
		{
			// Update of an existing silence with a non-matching label is denied.
			data: `{
    "id":"` + silID + `",
    "comment":"foo",
    "createdBy":"bar",
    "endsAt":"2020-02-13T13:00:02.084Z",
    "matchers": [
        {"isRegex":false,"Name":"foo","Value":"bar"}
    ],
    "startsAt":"2020-02-13T12:02:01Z"
}`,
			labelv: []string{"default"},
			upstream: &chainedHandlers{
				handlers: []http.Handler{
					getSilenceWithLabel("not default", false),
					createSilenceWithLabel("default", 1),
				},
			},

			expCode: http.StatusForbidden,
		},
		{
			// Update of a non-existing silence fails.
			data: `{
    "id":"does not exist",
    "comment":"foo",
    "createdBy":"bar",
    "endsAt":"2020-02-13T13:00:02.084Z",
    "matchers": [
        {"isRegex":false,"Name":"foo","Value":"bar"}
    ],
    "startsAt":"2020-02-13T12:02:01Z"
}`,
			labelv: []string{"default"},
			upstream: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				http.NotFound(w, req)
			}),

			expCode: http.StatusBadGateway,
		},
		{
			// The silence has the expected value for the label but upstream returns an error.
			data: `{
    "id":"` + silID + `",
    "comment":"foo",
    "createdBy":"bar",
    "endsAt":"2020-02-13T13:00:02.084Z",
    "matchers": [
        {"isRegex":false,"Name":"foo","Value":"bar"}
    ],
    "startsAt":"2020-02-13T12:02:01Z"
}`,
			labelv: []string{"default"},
			upstream: &chainedHandlers{
				handlers: []http.Handler{
					getSilenceWithLabel("default", true),
					http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
						w.WriteHeader(http.StatusTeapot)
					}),
				},
			},
			expCode: http.StatusTeapot,
		},
	} {
		t.Run("", func(t *testing.T) {
			m := newMockUpstream(tc.upstream)
			defer m.Close()
			r, err := NewRoutes(m.url, proxyLabel, HTTPFormEnforcer{ParameterName: proxyLabel})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			u, err := url.Parse("http://alertmanager.example.com/api/v2/silences/")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			q := u.Query()
			for _, lv := range tc.labelv {
				q.Add(proxyLabel, lv)
			}

			u.RawQuery = q.Encode()

			w := httptest.NewRecorder()
			req := httptest.NewRequest("POST", u.String(), bytes.NewBufferString(tc.data))
			r.ServeHTTP(w, req)

			resp := w.Result()
			body, _ := io.ReadAll(resp.Body)
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
func TestGetAlertGroups(t *testing.T) {
	for _, tc := range []struct {
		labelv         []string
		filters        []string
		expCode        int
		expQueryValues []string
		queryParam     string
		url            string
	}{
		{
			// No "namespace" parameter returns an error.
			expCode: http.StatusBadRequest,
			url:     "http://alertmanager.example.com/api/v2/alerts/groups",
		},
		{
			// Check for other query parameters
			labelv:         []string{"default"},
			expCode:        http.StatusOK,
			expQueryValues: []string{"false"},
			queryParam:     "silenced",
			url:            "http://alertmanager.example.com/api/v2/alerts/groups?silenced=false",
		},
		{
			// Check for filter parameter.
			labelv:         []string{"default"},
			filters:        []string{`job="prometheus"`, `instance=~".+"`},
			expCode:        http.StatusOK,
			expQueryValues: []string{`job="prometheus"`, `instance=~".+"`, `namespace=~"default"`},
			queryParam:     "filter",
			url:            "http://alertmanager.example.com/api/v2/alerts/groups",
		},
		{
			// Check for filter parameter with multiple label values.
			labelv:         []string{"default", "something"},
			filters:        []string{`job="prometheus"`, `instance=~".+"`},
			expCode:        http.StatusOK,
			expQueryValues: []string{`job="prometheus"`, `instance=~".+"`, `namespace=~"default|something"`},
			queryParam:     "filter",
			url:            "http://alertmanager.example.com/api/v2/alerts/groups",
		},
	} {
		t.Run(strings.Join(tc.filters, "&"), func(t *testing.T) {
			m := newMockUpstream(checkQueryHandler("", tc.queryParam, tc.expQueryValues...))
			defer m.Close()
			r, err := NewRoutes(m.url, proxyLabel, HTTPFormEnforcer{ParameterName: proxyLabel})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			u, err := url.Parse(tc.url)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			q := u.Query()
			for _, m := range tc.filters {
				q.Add("filter", m)
			}
			for _, lv := range tc.labelv {
				q.Add(proxyLabel, lv)
			}

			u.RawQuery = q.Encode()

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", u.String(), nil)
			r.ServeHTTP(w, req)

			resp := w.Result()
			body, _ := io.ReadAll(resp.Body)
			defer resp.Body.Close()

			if resp.StatusCode != tc.expCode {
				t.Logf("expected status code %d, got %d", tc.expCode, resp.StatusCode)
				t.Logf("%s", string(body))
				t.FailNow()
			}
			if resp.StatusCode != http.StatusOK {
				return
			}

		})
	}
}
