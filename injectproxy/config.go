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
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	"go.yaml.in/yaml/v3"
)

// Config declares the labels enforced by the proxy.
type Config struct {
	Labels []LabelConfig `yaml:"labels"`
}

// LabelConfig declares an enforced label and where its values come from.
// Exactly one of Header, QueryParam or Values must be set.
type LabelConfig struct {
	Name       string        `yaml:"name"`
	Header     *HeaderConfig `yaml:"header,omitempty"`
	QueryParam string        `yaml:"query_param,omitempty"`
	Values     []string      `yaml:"values,omitempty"`
}

// HeaderConfig declares the HTTP header carrying the label values.
type HeaderConfig struct {
	Name           string `yaml:"name"`
	UsesListSyntax bool   `yaml:"uses_list_syntax,omitempty"`
}

// LoadConfig reads and validates the configuration from the given file.
func LoadConfig(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("can't read the configuration file: %w", err)
	}

	cfg, err := parseConfig(b)
	if err != nil {
		return nil, fmt.Errorf("invalid configuration file %q: %w", path, err)
	}

	return cfg, nil
}

func parseConfig(b []byte) (*Config, error) {
	var cfg Config

	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("the configuration is empty")
		}

		return nil, err
	}

	if len(cfg.Labels) == 0 {
		return nil, errors.New("at least one label must be configured")
	}

	for i, l := range cfg.Labels {
		if err := l.validate(); err != nil {
			return nil, fmt.Errorf("labels[%d]: %w", i, err)
		}
	}

	return &cfg, nil
}

func (l LabelConfig) validate() error {
	if l.Name == "" {
		return errors.New("the label name can't be empty")
	}

	var sources int
	if l.Header != nil {
		sources++
	}
	if l.QueryParam != "" {
		sources++
	}
	if len(l.Values) > 0 {
		sources++
	}

	if sources != 1 {
		return fmt.Errorf("exactly one of header, query_param or values must be set for label %q, got %d", l.Name, sources)
	}

	if l.Header != nil && l.Header.Name == "" {
		return fmt.Errorf("the header name can't be empty for label %q", l.Name)
	}

	return nil
}

// LabelEnforcers returns the label enforcers declared by the configuration.
func (c Config) LabelEnforcers() []LabelEnforcer {
	enforcers := make([]LabelEnforcer, 0, len(c.Labels))
	for _, l := range c.Labels {
		enforcers = append(enforcers, LabelEnforcer{Label: l.Name, ExtractLabeler: l.extractLabeler()})
	}

	return enforcers
}

func (l LabelConfig) extractLabeler() ExtractLabeler {
	switch {
	case l.Header != nil:
		return HTTPHeaderEnforcer{
			Name:            http.CanonicalHeaderKey(l.Header.Name),
			ParseListSyntax: l.Header.UsesListSyntax,
		}
	case len(l.Values) > 0:
		return StaticLabelEnforcer(l.Values)
	default:
		return HTTPFormEnforcer{ParameterName: l.QueryParam}
	}
}
