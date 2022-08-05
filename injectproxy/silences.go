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
	"io/ioutil"
	"net/http"
	"path"
	"strconv"
	"strings"

	runtimeclient "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
	"github.com/pkg/errors"
	"github.com/prometheus/alertmanager/api/v2/client"
	"github.com/prometheus/alertmanager/api/v2/client/silence"
	"github.com/prometheus/alertmanager/api/v2/models"
	"github.com/prometheus/alertmanager/pkg/labels"
)

func (r *routes) silences(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case "GET":
		r.passthrough(w, req)
	case "POST":
		r.postSilence(w, req)
	default:
		http.NotFound(w, req)
	}
}

func (r *routes) filterSilences() func(*http.Response) error {
	return func(resp *http.Response) error {
		if resp.Request.Method == http.MethodPost {
			return nil
		}

		if resp.StatusCode != http.StatusOK {
			// Pass non-200 responses as-is.
			return nil
		}

		apir, err := getAlertmanagerAPIResponse(resp.Body)
		if err != nil {
			return errors.Wrap(err, "can't decode API response")
		}

		v, err := r.filterSilencesFromResp(MustLabelValues(resp.Request.Context()), apir)
		if err != nil {
			return err
		}

		var buf bytes.Buffer
		if err = json.NewEncoder(&buf).Encode(v); err != nil {
			return errors.Wrap(err, "can't encode API response")
		}
		resp.Body = ioutil.NopCloser(&buf)
		resp.Header["Content-Length"] = []string{fmt.Sprint(buf.Len())}

		return nil
	}
}

func (r *routes) filterSilencesFromResp(lvalues []string, data models.GettableSilences) (interface{}, error) {
	filtered := models.GettableSilences{}

	for _, gts := range data {
		if hasMatcherForLabel(gts.Matchers, r.label, lvalues) {
			filtered = append(filtered, gts)
		}
	}

	return filtered, nil
}

func getAlertmanagerAPIResponse(body io.ReadCloser) (models.GettableSilences, error) {
	defer body.Close()

	var apir models.GettableSilences
	if err := json.NewDecoder(body).Decode(&apir); err != nil {
		return nil, errors.Wrap(err, "JSON decoding")
	}

	return apir, nil
}

func (r *routes) enforceFilterParameter(w http.ResponseWriter, req *http.Request) {
	var (
		q               = req.URL.Query()
		proxyLabelMatch = labels.Matcher{
			Type:  labels.MatchRegexp,
			Name:  r.label,
			Value: joinMultipleLabelValues(MustLabelValues(req.Context())),
		}
		modified = []string{proxyLabelMatch.String()}
	)
	for _, filter := range q["filter"] {
		m, err := labels.ParseMatcher(filter)
		if err != nil {
			prometheusAPIError(w, fmt.Sprintf("bad request: can't parse filter %q: %v", filter, err), http.StatusBadRequest)
			return
		}
		if m.Name == r.label {
			continue
		}
		modified = append(modified, filter)
	}

	q["filter"] = modified
	q.Del(r.label)
	req.URL.RawQuery = q.Encode()

	r.handler.ServeHTTP(w, req)
}

func (r *routes) postSilence(w http.ResponseWriter, req *http.Request) {
	var (
		sil          models.PostableSilence
		lvalues      = MustLabelValues(req.Context())
		matcherValue = joinMultipleLabelValues(lvalues)
	)
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

		if !hasMatcherForLabel(existing.Matchers, r.label, lvalues) {
			prometheusAPIError(w, "forbidden", http.StatusForbidden)
			return
		}
	}

	truthy := true
	modified := models.Matchers{
		&models.Matcher{Name: &(r.label), Value: &matcherValue, IsRegex: &truthy},
	}
	for _, m := range sil.Matchers {
		modified = append(modified, m)
	}
	// At least one matcher in addition to the enforced label is required,
	// otherwise all alerts would be silenced
	if len(modified) < 2 {
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

	if !hasMatcherForLabel(sil.Matchers, r.label, MustLabelValues(req.Context())) {
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

func hasMatcherForLabel(matchers models.Matchers, name string, values []string) bool {
	for _, m := range matchers {
		if *m.Name == name {
			return *m.IsRegex && *m.Value == joinMultipleLabelValues(values)
		}
	}
	return false
}
