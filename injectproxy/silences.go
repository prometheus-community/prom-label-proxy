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
	"fmt"
	"io"
	"net/http"
	"path"
	"regexp"
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

// assertSingleLabelValue verifies that each enforced label has only one value.
// If not, it will reply with "422 Unprocessable Content".
func assertSingleLabelValue(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if labelValues, ok := req.Context().Value(keyLabels).(map[string][]string); ok {
			for _, values := range labelValues {
				if len(values) > 1 {
					http.Error(w, "Multiple label matchers not supported", http.StatusUnprocessableEntity)
					return
				}
			}
		} else if len(MustLabelValues(req.Context())) > 1 {
			http.Error(w, "Multiple label matchers not supported", http.StatusUnprocessableEntity)
			return
		}

		next(w, req)
	}
}

// enforceFilterParameter injects a label matcher parameter into the
// Alertmanager API's query.
func (r *routes) enforceFilterParameter(w http.ResponseWriter, req *http.Request) {
	proxyLabelMatches, err := r.newAlertmanagerLabelMatchers(req.Context())
	if err != nil {
		prometheusAPIError(w, err.Error(), http.StatusBadRequest)
		return
	}

	q := req.URL.Query()
	modified := make([]string, 0, len(proxyLabelMatches)+len(q["filter"]))
	for _, matcher := range proxyLabelMatches {
		modified = append(modified, matcher.String())
	}
	for _, filter := range q["filter"] {
		m, err := labels.ParseMatcher(filter)
		if err != nil {
			prometheusAPIError(w, fmt.Sprintf("bad request: can't parse filter %q: %v", filter, err), http.StatusBadRequest)
			return
		}

		drop := false
		for _, enforcedMatcher := range proxyLabelMatches {
			// Keep the original matcher in case of multi label values because
			// the user might want to filter on a specific value.
			if m.Name == enforcedMatcher.Name && enforcedMatcher.Type != labels.MatchRegexp {
				drop = true
				break
			}
		}
		if !drop {
			modified = append(modified, filter)
		}
	}

	q["filter"] = modified
	for _, config := range r.labels {
		q.Del(config.name)
	}
	req.URL.RawQuery = q.Encode()

	r.handler.ServeHTTP(w, req)
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

		if !r.hasEnforcedMatchers(existing.Matchers, req.Context()) {
			prometheusAPIError(w, "forbidden", http.StatusForbidden)
			return
		}
	}

	var falsy bool
	modified := make(models.Matchers, 0, len(r.labels)+len(sil.Matchers))
	for _, config := range r.labels {
		name := config.name
		value := mustLabelValuesFor(req.Context(), name)[0]
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

	if !r.hasEnforcedMatchers(sil.Matchers, req.Context()) {
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
		if m.Name != nil && m.IsRegex != nil && m.Value != nil && *m.Name == name && !*m.IsRegex && *m.Value == value {
			return true
		}
	}
	return false
}

func (r *routes) newAlertmanagerLabelMatchers(ctx context.Context) ([]labels.Matcher, error) {
	matchers := make([]labels.Matcher, 0, len(r.labels))
	for _, config := range r.labels {
		values := mustLabelValuesFor(ctx, config.name)
		matcher := labels.Matcher{Name: config.name}
		switch {
		case r.regexMatch:
			if len(values) != 1 {
				return nil, fmt.Errorf("only one label value allowed with regex match")
			}
			compiledRegex, err := regexp.Compile(values[0])
			if err != nil {
				return nil, fmt.Errorf("invalid regex: %w", err)
			}
			if compiledRegex.MatchString("") {
				return nil, fmt.Errorf("regex should not match empty string")
			}
			matcher.Type = labels.MatchRegexp
			matcher.Value = values[0]
		case len(values) > 1:
			matcher.Type = labels.MatchRegexp
			matcher.Value = labelValuesToRegexpString(values)
		default:
			matcher.Type = labels.MatchEqual
			matcher.Value = values[0]
		}
		matchers = append(matchers, matcher)
	}

	return matchers, nil
}

func (r *routes) isEnforcedLabel(name string) bool {
	for _, config := range r.labels {
		if config.name == name {
			return true
		}
	}
	return false
}

func (r *routes) hasEnforcedMatchers(matchers models.Matchers, ctx context.Context) bool {
	for _, config := range r.labels {
		if !hasMatcherForLabel(matchers, config.name, mustLabelValuesFor(ctx, config.name)[0]) {
			return false
		}
	}
	return true
}
