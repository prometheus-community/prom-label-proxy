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

func TestListSilences(t *testing.T) {
	for _, tc := range []struct {
		labelv     []string
		filters    []string
		regexMatch bool

		expCode    int
		expFilters []string
		expBody    []byte
	}{
		{
			// No "namespace" parameter returns an error.
			expCode: http.StatusBadRequest,
		},
		{
			// No "filter" parameter.
			labelv:     []string{"default"},
			expCode:    http.StatusOK,
			expFilters: []string{`namespace="default"`},
			expBody:    okResponse,
		},
		{
			// Many "filter" parameters.
			labelv:     []string{"default"},
			filters:    []string{`job="prometheus"`, `instance=~".+"`},
			expCode:    http.StatusOK,
			expFilters: []string{`job="prometheus"`, `instance=~".+"`, `namespace="default"`},
			expBody:    okResponse,
		},
		{
			// Many "filter" parameters with a "namespace" label that needs to be enforced.
			labelv:     []string{"default"},
			filters:    []string{`namespace=~"foo|default"`, `job="prometheus"`},
			expCode:    http.StatusOK,
			expFilters: []string{`namespace="default"`, `job="prometheus"`},
			expBody:    okResponse,
		},
		{
			// Invalid "filter" parameter.
			labelv:  []string{"default"},
			filters: []string{`namespace=~"foo|default"`, `job="promethe`},
			expCode: http.StatusBadRequest,
		},
		{
			// Multiple label values are not supported.
			labelv:  []string{"default", "something"},
			expCode: http.StatusUnprocessableEntity,
		},
		{
			// Regex match
			labelv:     []string{"tenant1-.*"},
			regexMatch: true,
			filters:    []string{`namespace=~"foo|default"`, `job="prometheus"`},
			expCode:    http.StatusNotImplemented,
		},
	} {
		t.Run(strings.Join(tc.filters, "&"), func(t *testing.T) {
			m := newMockUpstream(checkQueryHandler("", "filter", tc.expFilters...))
			defer m.Close()
			var opts []Option
			if tc.regexMatch {
				opts = append(opts, WithRegexMatch())
			}
			r, err := NewRoutes(m.url, proxyLabel, HTTPFormEnforcer{ParameterName: proxyLabel}, opts...)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			u, err := url.Parse("http://alertmanager.example.com/api/v2/silences")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			q := u.Query()
			for _, m := range tc.filters {
				q.Add("filter", m)
			}
			for _, s := range tc.labelv {
				q.Add(proxyLabel, s)
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

			if string(body) != string(tc.expBody) {
				t.Fatalf("expected body %q, got %q", string(tc.expBody), string(body))
			}
		})
	}
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

func getSilenceWithLabel(labelv string) http.Handler {
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
      "name": "%s",
      "value": "%s"
    }
  ],
  "startsAt": "2020-02-13T12:02:01.000Z"
}
				`, silID, proxyLabel, labelv)
	})
}

func createSilenceWithLabel(labelv string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var sil models.PostableSilence
		if err := json.NewDecoder(req.Body).Decode(&sil); err != nil {
			prometheusAPIError(w, fmt.Sprintf("unexpected error: %v", err), http.StatusInternalServerError)
			return
		}
		var values []string
		for _, m := range sil.Matchers {
			if *m.Name == proxyLabel {
				values = append(values, *m.Value)
			}
		}
		if len(values) != 1 {
			prometheusAPIError(w, fmt.Sprintf("expected 1 matcher for label %s, got %d", proxyLabel, len(values)), http.StatusInternalServerError)
			return
		}
		if values[0] != labelv {
			prometheusAPIError(w, fmt.Sprintf("expected matcher for label %s to be %q, got %q", proxyLabel, labelv, values[0]), http.StatusInternalServerError)
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
		ID         string
		labelv     []string
		upstream   http.Handler
		regexMatch bool

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
			upstream: getSilenceWithLabel("not default"),
			expCode:  http.StatusForbidden,
		},
		{
			// The silence has the expected value for the label.
			ID:     silID,
			labelv: []string{"default"},
			upstream: &chainedHandlers{
				handlers: []http.Handler{
					getSilenceWithLabel("default"),
					http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
						w.Write([]byte("ok"))
					}),
				},
			},
			expCode: http.StatusOK,
			expBody: []byte("ok"),
		},
		{
			// The silence has the expected value for the label but upstream returns an error.
			ID:     silID,
			labelv: []string{"default"},
			upstream: &chainedHandlers{
				handlers: []http.Handler{
					getSilenceWithLabel("default"),
					http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
						w.WriteHeader(http.StatusTeapot)
					}),
				},
			},
			expCode: http.StatusTeapot,
		},
		{
			// Multiple label values are not supported.
			labelv:  []string{"default", "something"},
			expCode: http.StatusUnprocessableEntity,
		},
		{
			// Regexp is not supported.
			labelv:     []string{"default"},
			regexMatch: true,
			expCode:    http.StatusNotImplemented,
		},
	} {
		t.Run("", func(t *testing.T) {
			m := newMockUpstream(tc.upstream)
			defer m.Close()
			var opts []Option
			if tc.regexMatch {
				opts = append(opts, WithRegexMatch())
			}
			r, err := NewRoutes(m.url, proxyLabel, HTTPFormEnforcer{ParameterName: proxyLabel}, opts...)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			u, err := url.Parse(fmt.Sprintf("http://alertmanager.example.com/api/v2/silence/%s", tc.ID))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			q := u.Query()
			for _, s := range tc.labelv {
				q.Add(proxyLabel, s)
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
			upstream: createSilenceWithLabel("default"),

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
			upstream: createSilenceWithLabel("default"),

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
					getSilenceWithLabel("default"),
					createSilenceWithLabel("default"),
				},
			},

			expCode: http.StatusOK,
			expBody: okResponse,
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
					getSilenceWithLabel("not default"),
					createSilenceWithLabel("default"),
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
					getSilenceWithLabel("default"),
					http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
						w.WriteHeader(http.StatusTeapot)
					}),
				},
			},
			expCode: http.StatusTeapot,
		},
		{
			// Multiple label values are not supported.
			labelv:  []string{"default", "something"},
			expCode: http.StatusUnprocessableEntity,
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
			for _, s := range tc.labelv {
				q.Add(proxyLabel, s)
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
			expQueryValues: []string{`job="prometheus"`, `instance=~".+"`, `namespace="default"`},
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
			for _, s := range tc.labelv {
				q.Add(proxyLabel, s)
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
