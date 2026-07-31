// Copyright 2026 The Prometheus Authors
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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseConfig(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string

		want    *Config
		wantErr bool
	}{
		{
			name: "all value sources",
			in: `
labels:
  - name: namespace
    header:
      name: X-Namespace
      uses_list_syntax: true
  - name: cluster
    query_param: cluster
  - name: tenant
    values:
      - team-a
      - team-b
`,
			want: &Config{
				Labels: []LabelConfig{
					{Name: "namespace", Header: &HeaderConfig{Name: "X-Namespace", UsesListSyntax: true}},
					{Name: "cluster", QueryParam: "cluster"},
					{Name: "tenant", Values: []string{"team-a", "team-b"}},
				},
			},
		},
		{
			name:    "empty document",
			in:      "",
			wantErr: true,
		},
		{
			name:    "no labels",
			in:      "labels: []\n",
			wantErr: true,
		},
		{
			name:    "unknown field",
			in:      "labels:\n  - name: namespace\n    query_param: ns\n    foo: bar\n",
			wantErr: true,
		},
		{
			name:    "missing label name",
			in:      "labels:\n  - query_param: ns\n",
			wantErr: true,
		},
		{
			name:    "no value source",
			in:      "labels:\n  - name: namespace\n",
			wantErr: true,
		},
		{
			name:    "several value sources",
			in:      "labels:\n  - name: namespace\n    query_param: ns\n    values: [team-a]\n",
			wantErr: true,
		},
		{
			name:    "empty header name",
			in:      "labels:\n  - name: namespace\n    header:\n      name: \"\"\n",
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseConfig([]byte(tc.in))
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("expected %+v, got %+v", tc.want, got)
			}
		})
	}
}

func TestLoadConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("labels:\n  - name: namespace\n    query_param: ns\n"), 0o600); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Labels) != 1 || cfg.Labels[0].QueryParam != "ns" {
		t.Fatalf("unexpected configuration: %+v", cfg)
	}

	if _, err := LoadConfig(filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Fatal("expected error, got none")
	}
}

func TestConfigLabelEnforcers(t *testing.T) {
	m := newMockUpstream(checkQueryHandler("", queryParam, `up{cluster="cluster-a",namespace="team-a",tenant="acme"}`))
	defer m.Close()

	cfg, err := parseConfig([]byte(`
labels:
  - name: namespace
    header:
      name: x-namespace
  - name: cluster
    query_param: cluster
  - name: tenant
    values: [acme]
`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, err := NewRoutesWithLabelers(m.url, cfg.LabelEnforcers())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://prometheus.example.com/api/v1/query?query=up&cluster=cluster-a", nil)
	req.Header.Set("X-Namespace", "team-a")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status code %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}
}

func TestNewRoutesWithoutLabel(t *testing.T) {
	m := newMockUpstream(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer m.Close()

	if _, err := NewRoutes(m.url, "", nil); err == nil {
		t.Fatal("expected error, got none")
	}
}
