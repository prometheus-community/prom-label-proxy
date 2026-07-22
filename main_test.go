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

package main

import "testing"

func TestValidateLabelFlags(t *testing.T) {
	for _, tc := range []struct {
		name        string
		labels      []string
		queryParams []string
		headerNames []string
		labelValues []string
		wantErr     string
	}{
		{
			name:        "single query parameter",
			labels:      []string{"namespace"},
			queryParams: []string{"namespace"},
		},
		{
			name:        "multiple query parameters",
			labels:      []string{"namespace", "cluster"},
			queryParams: []string{"namespace", "cluster"},
		},
		{
			name:        "multiple headers",
			labels:      []string{"namespace", "cluster"},
			headerNames: []string{"X-Namespace", "X-Cluster"},
		},
		{
			name:        "multiple static values for one label",
			labels:      []string{"namespace"},
			labelValues: []string{"team-a", "team-b"},
		},
		{
			name:    "missing label",
			wantErr: "-label flag cannot be empty",
		},
		{
			name:    "empty label",
			labels:  []string{"namespace", ""},
			wantErr: "-label flag cannot be empty",
		},
		{
			name:    "duplicate label",
			labels:  []string{"namespace", "namespace"},
			wantErr: `-label "namespace" is configured more than once`,
		},
		{
			name:        "static and query parameter sources",
			labels:      []string{"namespace"},
			queryParams: []string{"namespace"},
			labelValues: []string{"team-a"},
			wantErr:     "-label-value cannot be combined with -query-param or -header-name",
		},
		{
			name:        "static values for multiple labels",
			labels:      []string{"namespace", "cluster"},
			labelValues: []string{"team-a"},
			wantErr:     "-label-value cannot be used with multiple -label flags",
		},
		{
			name:        "mixed dynamic sources",
			labels:      []string{"namespace"},
			queryParams: []string{"namespace"},
			headerNames: []string{"X-Namespace"},
			wantErr:     "-query-param and -header-name cannot be combined",
		},
		{
			name:        "query parameter count mismatch",
			labels:      []string{"namespace", "cluster"},
			queryParams: []string{"namespace"},
			wantErr:     "the number of -query-param flags must match the number of -label flags",
		},
		{
			name:        "header count mismatch",
			labels:      []string{"namespace", "cluster"},
			headerNames: []string{"X-Namespace"},
			wantErr:     "the number of -header-name flags must match the number of -label flags",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateLabelFlags(tc.labels, tc.queryParams, tc.headerNames, tc.labelValues)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || err.Error() != tc.wantErr {
				t.Fatalf("expected error %q, got %v", tc.wantErr, err)
			}
		})
	}
}
