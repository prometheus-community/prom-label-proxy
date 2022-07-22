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
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestGetAlerts(t *testing.T) {
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
			url:     "http://alertmanager.example.com/api/v2/alerts",
		},
		{
			// Check that other query parameters are not removed.
			labelv:         []string{"default"},
			expCode:        http.StatusOK,
			expQueryValues: []string{"false"},
			queryParam:     "silenced",
			url:            "http://alertmanager.example.com/api/v2/alerts?silenced=false",
		},
		{
			// Check that filter parameter is added when other query parameter are present
			labelv:         []string{"default"},
			expCode:        http.StatusOK,
			expQueryValues: []string{`namespace=~"default"`},
			queryParam:     "filter",
			url:            "http://alertmanager.example.com/api/v2/alerts?silenced=false",
		},
		{
			// Check that filter parameter is added when multiple label values are set
			labelv:         []string{"default", "something"},
			expCode:        http.StatusOK,
			expQueryValues: []string{`namespace=~"default|something"`},
			queryParam:     "filter",
			url:            "http://alertmanager.example.com/api/v2/alerts?silenced=false",
		},
		{
			// Check that label values are correctly escaped
			labelv:         []string{"default", "some|thing"},
			expCode:        http.StatusOK,
			expQueryValues: []string{`namespace=~"default|some\\|thing"`},
			queryParam:     "filter",
			url:            "http://alertmanager.example.com/api/v2/alerts?silenced=false",
		},
		{
			// Check for filter parameter.
			labelv:         []string{"default"},
			filters:        []string{`job="prometheus"`, `instance=~".+"`},
			expCode:        http.StatusOK,
			expQueryValues: []string{`job="prometheus"`, `instance=~".+"`, `namespace=~"default"`},
			queryParam:     "filter",
			url:            "http://alertmanager.example.com/api/v2/alerts",
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
