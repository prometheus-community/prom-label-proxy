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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	runtimeclient "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
	"github.com/prometheus/alertmanager/api/v2/client"
	"github.com/prometheus/alertmanager/api/v2/client/silence"
	"github.com/prometheus/alertmanager/api/v2/models"
	"github.com/prometheus/alertmanager/pkg/labels"
)

// silences proxies HTTP requests to the Alertmanager /api/v2/silences endpoint.
func (r *routes) silences(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case "GET":
		r.enforceFilterParameter(w, req)
	case "POST":
		r.postSilence(w, req)
	default:
		http.NotFound(w, req)
	}
}

// assertSingleLabelValue verifies that the proxy is configured to match only
// one value for each enforced label. If not, it will reply with "422
// Unprocessable Content".
func (r *routes) assertSingleLabelValue(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		for _, l := range r.labels {
			if len(mustLabelValuesFor(req.Context(), l.Label)) > 1 {
				http.Error(w, "Multiple label matchers not supported", http.StatusUnprocessableEntity)
				return
			}
		}

		next(w, req)
	}
}

// enforceFilterParameter injects a label matcher parameter into the
// Alertmanager API's query.
func (r *routes) enforceFilterParameter(w http.ResponseWriter, req *http.Request) {
	proxyLabelMatchers, err := r.newAlertmanagerMatchers(req.Context())
	if err != nil {
		prometheusAPIError(w, err.Error(), http.StatusBadRequest)
		return
	}

	q := req.URL.Query()
	modified := make([]string, 0, len(proxyLabelMatchers)+len(q["filter"]))
	for _, m := range proxyLabelMatchers {
		modified = append(modified, m.String())
	}

	for _, filter := range q["filter"] {
		m, err := labels.ParseMatcher(filter)
		if err != nil {
			prometheusAPIError(w, fmt.Sprintf("bad request: can't parse filter %q: %v", filter, err), http.StatusBadRequest)
			return
		}

		// Keep the original matcher in case of multi label values because
		// the user might want to filter on a specific value.
		if slices.ContainsFunc(proxyLabelMatchers, func(pm labels.Matcher) bool {
			return pm.Name == m.Name && pm.Type != labels.MatchRegexp
		}) {
			continue
		}

		modified = append(modified, filter)
	}

	q["filter"] = modified
	for _, l := range r.labels {
		q.Del(l.Label)
	}
	req.URL.RawQuery = q.Encode()

	r.handler.ServeHTTP(w, req)
}

// newAlertmanagerMatchers returns one Alertmanager matcher per enforced label.
func (r *routes) newAlertmanagerMatchers(ctx context.Context) ([]labels.Matcher, error) {
	matchers := make([]labels.Matcher, 0, len(r.labels))
	for _, l := range r.labels {
		values := mustLabelValuesFor(ctx, l.Label)

		m := labels.Matcher{Type: labels.MatchEqual, Name: l.Label, Value: values[0]}
		switch {
		case r.regexMatch:
			compiledRegex, err := regexp.Compile(values[0])
			if err != nil {
				return nil, err
			}
			if compiledRegex.MatchString("") {
				return nil, errors.New("regex should not match empty string")
			}
			m.Type = labels.MatchRegexp
		case len(values) > 1:
			m.Type = labels.MatchRegexp
			m.Value = labelValuesToRegexpString(values)
		}

		matchers = append(matchers, m)
	}

	return matchers, nil
}

func (r *routes) postSilence(w http.ResponseWriter, req *http.Request) {
	var sil models.PostableSilence

	if err := json.NewDecoder(req.Body).Decode(&sil); err != nil {
		prometheusAPIError(w, fmt.Sprintf("bad request: can't decode: %v", err), http.StatusBadRequest)
		return
	}

	if sil.ID != "" {
		// This is an update for an existing silence.
		existing, err := r.getSilenceByID(req.Context(), sil.ID)
		if err != nil {
			prometheusAPIError(w, fmt.Sprintf("proxy error: can't get silence: %v", err), http.StatusBadGateway)
			return
		}

		if !r.hasEnforcedMatchers(req.Context(), existing.Matchers) {
			prometheusAPIError(w, "forbidden", http.StatusForbidden)
			return
		}
	}

	var falsy bool
	modified := make(models.Matchers, 0, len(r.labels)+len(sil.Matchers))
	for _, l := range r.labels {
		// Single value guaranteed by assertSingleLabelValue().
		name, value := l.Label, mustLabelValuesFor(req.Context(), l.Label)[0]
		modified = append(modified, &models.Matcher{Name: &name, Value: &value, IsRegex: &falsy})
	}
	for _, m := range sil.Matchers {
		if m.Name != nil && r.isEnforcedLabel(*m.Name) {
			continue
		}
		modified = append(modified, m)
	}
	// At least one matcher in addition to the enforced labels is required,
	// otherwise all alerts would be silenced
	if len(modified) == len(r.labels) {
		prometheusAPIError(w, "need at least one matcher, got none", http.StatusBadRequest)
		return
	}
	sil.Matchers = modified

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(&sil); err != nil {
		prometheusAPIError(w, fmt.Sprintf("can't encode: %v", err), http.StatusInternalServerError)
		return
	}

	req = req.Clone(req.Context())
	req.Body = io.NopCloser(&buf)
	req.URL.RawQuery = ""
	req.Header["Content-Length"] = []string{strconv.Itoa(buf.Len())}
	req.ContentLength = int64(buf.Len())

	r.handler.ServeHTTP(w, req)
}

// deleteSilence proxies HTTP requests to the Alertmanager /api/v2/silence/ endpoint.
func (r *routes) deleteSilence(w http.ResponseWriter, req *http.Request) {
	silID := strings.TrimPrefix(req.URL.Path, "/api/v2/silence/")
	if silID == "" || silID == req.URL.Path {
		prometheusAPIError(w, "bad request", http.StatusBadRequest)
		return
	}

	// Get the silence by ID and verify that it has the expected label.
	sil, err := r.getSilenceByID(req.Context(), silID)
	if err != nil {
		prometheusAPIError(w, fmt.Sprintf("proxy error: %v", err), http.StatusBadGateway)
		return
	}

	if !r.hasEnforcedMatchers(req.Context(), sil.Matchers) {
		prometheusAPIError(w, "forbidden", http.StatusForbidden)
		return
	}

	req.URL.RawQuery = ""
	r.handler.ServeHTTP(w, req)
}

func (r *routes) getSilenceByID(ctx context.Context, id string) (*models.GettableSilence, error) {
	amc := client.New(
		runtimeclient.New(r.upstream.Host, path.Join(r.upstream.Path, "/api/v2"), []string{r.upstream.Scheme}),
		strfmt.Default,
	)
	params := silence.NewGetSilenceParams().WithContext(ctx)
	params.SetSilenceID(strfmt.UUID(id))
	sil, err := amc.Silence.GetSilence(params)
	if err != nil {
		return nil, err
	}
	return sil.Payload, nil
}

func hasMatcherForLabel(matchers models.Matchers, name, value string) bool {
	for _, m := range matchers {
		if *m.Name == name && !*m.IsRegex && *m.Value == value {
			return true
		}
	}
	return false
}

func (r *routes) isEnforcedLabel(name string) bool {
	return slices.ContainsFunc(r.labels, func(l LabelEnforcer) bool { return l.Label == name })
}

// hasEnforcedMatchers returns true when the silence has a matcher for every enforced label.
func (r *routes) hasEnforcedMatchers(ctx context.Context, matchers models.Matchers) bool {
	for _, l := range r.labels {
		// Single value guaranteed by assertSingleLabelValue().
		if !hasMatcherForLabel(matchers, l.Label, mustLabelValuesFor(ctx, l.Label)[0]) {
			return false
		}
	}

	return true
}
