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
	"errors"
	"fmt"
	"testing"

	"github.com/prometheus/prometheus/model/labels"
)

func mustNewMatcher(t labels.MatchType, n, v string) *labels.Matcher {
	m, err := labels.NewMatcher(t, n, v)
	if err != nil {
		panic(err)
	}

	return m
}

type checkFunc func(expression string, err error) error

func checks(cs ...checkFunc) checkFunc {
	return func(expression string, err error) error {
		for _, c := range cs {
			if e := c(expression, err); e != nil {
				return e
			}
		}
		return nil
	}
}

func noError() checkFunc {
	return func(_ string, got error) error {
		if got != nil {
			return fmt.Errorf("want error <nil>, got %v", got)
		}

		return nil
	}
}

func errorIs(want error) checkFunc {
	return func(_ string, got error) error {
		if errors.Is(got, want) {
			return nil
		}
		return fmt.Errorf("want error of type %T, got %v", want, got)
	}
}

func hasExpression(want string) checkFunc {
	return func(got string, _ error) error {
		if want != got {
			return fmt.Errorf("want expression \n%v\ngot \n%v", want, got)
		}
		return nil
	}
}

var tests = []struct {
	name       string
	expression string
	enforcer   *PromQLEnforcer
	check      checkFunc
}{
	// first check correct label insertion
	{
		name:       "expressions add label",
		expression: `round(metric1{label="baz"},3)`,
		enforcer: NewPromQLEnforcer(
			false,
			&labels.Matcher{
				Name:  "namespace",
				Type:  labels.MatchEqual,
				Value: "NS",
			},
			&labels.Matcher{
				Name:  "pod",
				Type:  labels.MatchEqual,
				Value: "POD",
			},
		),
		check: checks(
			noError(),
			hasExpression(`round(metric1{label="baz",namespace="NS",pod="POD"}, 3)`),
		),
	},

	{
		name:       "aggregate add label",
		expression: `sum by (pod) (metric1{label="baz"})`,
		enforcer: NewPromQLEnforcer(
			false,
			&labels.Matcher{
				Name:  "namespace",
				Type:  labels.MatchEqual,
				Value: "NS",
			},
			&labels.Matcher{
				Name:  "pod",
				Type:  labels.MatchEqual,
				Value: "POD",
			},
		),
		check: checks(
			noError(),
			hasExpression(`sum by (pod) (metric1{label="baz",namespace="NS",pod="POD"})`),
		),
	},

	{
		name:       "binary expression add label",
		expression: `metric1{} + sum by (pod) (metric2{label="baz"})`,
		enforcer: NewPromQLEnforcer(
			false,
			&labels.Matcher{
				Name:  "namespace",
				Type:  labels.MatchEqual,
				Value: "NS",
			},
			&labels.Matcher{
				Name:  "pod",
				Type:  labels.MatchEqual,
				Value: "POD",
			},
		),
		check: checks(
			noError(),
			hasExpression(`metric1{namespace="NS",pod="POD"} + sum by (pod) (metric2{label="baz",namespace="NS",pod="POD"})`),
		),
	},

	{
		name:       "binary expression with vector matching add label",
		expression: `metric1{} + on(pod,namespace) sum by (pod) (metric2{label="baz"})`,
		enforcer: NewPromQLEnforcer(
			false,
			&labels.Matcher{
				Name:  "namespace",
				Type:  labels.MatchEqual,
				Value: "NS",
			},
			&labels.Matcher{
				Name:  "pod",
				Type:  labels.MatchEqual,
				Value: "POD",
			},
		),
		check: checks(
			noError(),
			hasExpression(`metric1{namespace="NS",pod="POD"} + on (pod, namespace) sum by (pod) (metric2{label="baz",namespace="NS",pod="POD"})`),
		),
	},
	// then check error return when a query would be silently altered, i.e. a label
	// matcher would be changed
	{
		name:       "expressions error on non-matching label value",
		expression: `round(metric1{label="baz",pod="POD",namespace="bar"},3)`,
		enforcer: NewPromQLEnforcer(
			true,
			&labels.Matcher{
				Name:  "namespace",
				Type:  labels.MatchEqual,
				Value: "NS",
			},
			&labels.Matcher{
				Name:  "pod",
				Type:  labels.MatchEqual,
				Value: "POD",
			},
		),
		check: checks(
			errorIs(ErrIllegalLabelMatcher),
		),
	},

	{
		name:       "aggregate error on non-matching label value",
		expression: `sum by (pod) (metric1{label="baz",pod="foo",namespace="bar"})`,
		enforcer: NewPromQLEnforcer(
			true,
			&labels.Matcher{
				Name:  "namespace",
				Type:  labels.MatchEqual,
				Value: "NS",
			},
			&labels.Matcher{
				Name:  "pod",
				Type:  labels.MatchEqual,
				Value: "POD",
			},
		),
		check: checks(
			errorIs(ErrIllegalLabelMatcher),
		),
	},

	{
		name:       "binary expression error on non-matching label value",
		expression: `metric1{pod="baz"} + sum by (pod) (metric2{label="baz",pod="foo",namespace="bar"})`,
		enforcer: NewPromQLEnforcer(
			true,
			&labels.Matcher{
				Name:  "namespace",
				Type:  labels.MatchEqual,
				Value: "NS",
			},
			&labels.Matcher{
				Name:  "pod",
				Type:  labels.MatchEqual,
				Value: "POD",
			},
		),
		check: checks(
			errorIs(ErrIllegalLabelMatcher),
		),
	},

	{
		name:       "binary expression with vector matching error on non-matching label value",
		expression: `metric1{pod="baz"} + on (pod,namespace) sum by (pod) (metric2{label="baz",pod="foo",namespace="bar"})`,
		enforcer: NewPromQLEnforcer(
			true,
			&labels.Matcher{
				Name:  "namespace",
				Type:  labels.MatchEqual,
				Value: "NS",
			},
			&labels.Matcher{
				Name:  "pod",
				Type:  labels.MatchEqual,
				Value: "POD",
			},
		),
		check: checks(
			errorIs(ErrIllegalLabelMatcher),
		),
	},
	// and lastly check that passing the label matcher we would inject
	// doesn't return an error
	{
		name:       "expressions unchanged with matching label value",
		expression: `round(metric1{label="baz",pod="POD",namespace="NS"},3)`,
		enforcer: NewPromQLEnforcer(
			false,
			&labels.Matcher{
				Name:  "namespace",
				Type:  labels.MatchEqual,
				Value: "NS",
			},
			&labels.Matcher{
				Name:  "pod",
				Type:  labels.MatchEqual,
				Value: "POD",
			},
		),
		check: checks(
			noError(),
			hasExpression(`round(metric1{label="baz",namespace="NS",pod="POD"}, 3)`),
		),
	},

	{
		name:       "aggregate unchanged with matching label value",
		expression: `sum by (pod) (metric1{label="baz",pod="POD",namespace="NS"})`,
		enforcer: NewPromQLEnforcer(
			false,
			&labels.Matcher{
				Name:  "namespace",
				Type:  labels.MatchEqual,
				Value: "NS",
			},
			&labels.Matcher{
				Name:  "pod",
				Type:  labels.MatchEqual,
				Value: "POD",
			},
		),
		check: checks(
			noError(),
			hasExpression(`sum by (pod) (metric1{label="baz",namespace="NS",pod="POD"})`),
		),
	},

	{
		name:       "binary expression unchanged with matching label value",
		expression: `metric1{pod="POD"} + sum by (pod) (metric2{label="baz",namespace="NS",pod="POD"})`,
		enforcer: NewPromQLEnforcer(
			false,
			&labels.Matcher{
				Name:  "namespace",
				Type:  labels.MatchEqual,
				Value: "NS",
			},
			&labels.Matcher{
				Name:  "pod",
				Type:  labels.MatchEqual,
				Value: "POD",
			},
		),
		check: checks(
			noError(),
			hasExpression(`metric1{namespace="NS",pod="POD"} + sum by (pod) (metric2{label="baz",namespace="NS",pod="POD"})`),
		),
	},

	{
		name:       "binary expression with vector matching unchanged with matching label value",
		expression: `metric1{pod="POD"} + on (pod,namespace) sum by (pod) (metric2{label="baz",pod="POD",namespace="NS"})`,
		enforcer: NewPromQLEnforcer(
			false,
			&labels.Matcher{
				Name:  "namespace",
				Type:  labels.MatchEqual,
				Value: "NS",
			},
			&labels.Matcher{
				Name:  "pod",
				Type:  labels.MatchEqual,
				Value: "POD",
			},
		),
		check: checks(
			noError(),
			hasExpression(`metric1{namespace="NS",pod="POD"} + on (pod, namespace) sum by (pod) (metric2{label="baz",namespace="NS",pod="POD"})`),
		),
	},
	{
		name:       "invalid PromQL expression",
		expression: `metric1{pod="baz"`,
		enforcer: NewPromQLEnforcer(
			false,
			&labels.Matcher{
				Name:  "namespace",
				Type:  labels.MatchEqual,
				Value: "NS",
			},
		),
		check: checks(
			errorIs(ErrQueryParse),
		),
	},
}

func TestEnforce(t *testing.T) {
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.enforcer.Enforce(tc.expression)
			if err := tc.check(got, err); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestEnforceWithErrOnReplace(t *testing.T) {
	type subTestCase struct {
		labelSelector string
		exp           string
		err           bool
	}

	for _, tc := range []struct {
		enforcedMatcher *labels.Matcher
		stcs            []subTestCase
	}{
		// Equal matcher enforcer.
		{
			enforcedMatcher: mustNewMatcher(labels.MatchEqual, "job", "foo"),
			stcs: []subTestCase{
				// No selector in the expression for the enforced label.
				{
					``,
					`up{job="foo"}`,
					false,
				},

				// Equal label selector in the expression.
				{
					`job=""`,
					``,
					true,
				},
				{
					`job="foo"`,
					`up{job="foo"}`,
					false,
				},
				{
					`job="bar"`,
					``,
					true,
				},
				{
					`job="fred"`,
					``,
					true,
				},

				// Not equal label selector in the expression.
				{
					`job!=""`,
					`up{job="foo"}`,
					false,
				},
				{
					`job!="foo"`,
					``,
					true,
				},
				{
					`job!="bar"`,
					`up{job="foo"}`,
					false,
				},
				{
					`job!="fred"`,
					`up{job="foo"}`,
					false,
				},

				// Regexp label selector in the expression.
				{
					`job=~""`,
					``,
					true,
				},
				{
					`job=~"foo"`,
					`up{job="foo"}`,
					false,
				},
				{
					`job=~"bar"`,
					``,
					true,
				},
				{
					`job=~"fred"`,
					``,
					true,
				},
				{
					`job=~"foo|fred"`,
					`up{job="foo"}`,
					false,
				},
				{
					`job=~"foo|bar"`,
					`up{job="foo"}`,
					false,
				},

				// Not-regexp label selector in the expression.
				{
					`job!~""`,
					`up{job="foo"}`,
					false,
				},
				{
					`job!~"foo"`,
					``,
					true,
				},
				{
					`job!~"bar"`,
					`up{job="foo"}`,
					false,
				},
				{
					`job!~"fred"`,
					`up{job="foo"}`,
					false,
				},
				{
					`job!~"foo|fred"`,
					``,
					true,
				},
				{
					`job!~"foo|bar"`,
					``,
					true,
				},
			},
		},

		// Not equal matcher enforcer.
		{
			enforcedMatcher: mustNewMatcher(labels.MatchNotEqual, "job", "foo"),
			stcs: []subTestCase{
				// No selector in the expression for the enforced label.
				{
					``,
					`up{job!="foo"}`,
					false,
				},

				// Equal label selector in the expression.
				{
					`job=""`,
					`up{job!="foo",job=""}`,
					false,
				},
				{
					`job="foo"`,
					``,
					true,
				},
				{
					`job="bar"`,
					`up{job!="foo",job="bar"}`,
					false,
				},
				{
					`job="fred"`,
					`up{job!="foo",job="fred"}`,
					false,
				},

				// Not equal label selector in the expression.
				{
					`job!=""`,
					`up{job!="",job!="foo"}`,
					false,
				},
				{
					`job!="foo"`,
					`up{job!="foo"}`,
					false,
				},
				{
					`job!="bar"`,
					`up{job!="bar",job!="foo"}`,
					false,
				},
				{
					`job!="fred"`,
					`up{job!="foo",job!="fred"}`,
					false,
				},

				// Regexp label selector in the expression.
				{
					`job=~""`,
					`up{job!="foo",job=~""}`,
					false,
				},
				{
					// up{job!="foo",job=~"foo"} would return no result.
					`job=~"foo"`,
					``,
					true,
				},
				{
					`job=~"bar"`,
					`up{job!="foo",job=~"bar"}`,
					false,
				},
				{
					`job=~"fred"`,
					`up{job!="foo",job=~"fred"}`,
					false,
				},
				{
					`job=~"foo|fred"`,
					`up{job!="foo",job=~"foo|fred"}`,
					false,
				},
				{
					`job=~"foo|bar"`,
					`up{job!="foo",job=~"foo|bar"}`,
					false,
				},

				// Not-regexp label selector in the expression.
				{
					`job!~""`,
					`up{job!="foo",job!~""}`,
					false,
				},
				{
					`job!~"foo"`,
					`up{job!="foo",job!~"foo"}`,
					false,
				},
				{
					`job!~"bar"`,
					`up{job!="foo",job!~"bar"}`,
					false,
				},
				{
					`job!~"fred"`,
					`up{job!="foo",job!~"fred"}`,
					false,
				},
				{
					`job!~"foo|fred"`,
					`up{job!="foo",job!~"foo|fred"}`,
					false,
				},
				{
					`job!~"foo|bar"`,
					`up{job!="foo",job!~"foo|bar"}`,
					false,
				},
			},
		},

		// Regexp matcher enforcer.
		{
			enforcedMatcher: mustNewMatcher(labels.MatchRegexp, "job", "foo|bar"),
			stcs: []subTestCase{
				// No selector in the expression for the enforced label.
				{
					``,
					`up{job=~"foo|bar"}`,
					false,
				},

				// Equal label selector in the expression.
				{
					`job=""`,
					``,
					true,
				},
				{
					`job="foo"`,
					`up{job="foo",job=~"foo|bar"}`,
					false,
				},
				{
					`job="bar"`,
					`up{job="bar",job=~"foo|bar"}`,
					false,
				},
				{
					`job="fred"`,
					``,
					true,
				},

				// Not equal label selector in the expression.
				{
					`job!=""`,
					`up{job!="",job=~"foo|bar"}`,
					false,
				},
				{
					`job!="foo"`,
					`up{job!="foo",job=~"foo|bar"}`,
					false,
				},
				{
					`job!="bar"`,
					`up{job!="bar",job=~"foo|bar"}`,
					false,
				},
				{
					`job!="fred"`,
					`up{job!="fred",job=~"foo|bar"}`,
					false,
				},

				// Regexp label selector in the expression.
				{
					`job=~""`,
					``,
					true,
				},
				{
					`job=~"foo"`,
					`up{job=~"foo",job=~"foo|bar"}`,
					false,
				},
				{
					`job=~"bar"`,
					`up{job=~"bar",job=~"foo|bar"}`,
					false,
				},
				{
					`job=~"fred"`,
					`up{job=~"foo|bar",job=~"fred"}`,
					false,
				},
				{
					`job=~"foo|fred"`,
					`up{job=~"foo|bar",job=~"foo|fred"}`,
					false,
				},
				{
					`job=~"foo|bar"`,
					`up{job=~"foo|bar"}`,
					false,
				},

				// Not-regexp label selector in the expression.
				{
					`job!~""`,
					`up{job!~"",job=~"foo|bar"}`,
					false,
				},
				{
					`job!~"foo"`,
					`up{job!~"foo",job=~"foo|bar"}`,
					false,
				},
				{
					`job!~"bar"`,
					`up{job!~"bar",job=~"foo|bar"}`,
					false,
				},
				{
					`job!~"fred"`,
					`up{job!~"fred",job=~"foo|bar"}`,
					false,
				},
				{
					`job!~"foo|fred"`,
					`up{job!~"foo|fred",job=~"foo|bar"}`,
					false,
				},
				{
					`job!~"foo|bar"`,
					``,
					true,
				},
			},
		},

		// Not regexp matcher enforcer.
		{
			enforcedMatcher: mustNewMatcher(labels.MatchNotRegexp, "job", "foo|bar"),
			stcs: []subTestCase{
				// No selector in the expression for the enforced label.
				{
					``,
					`up{job!~"foo|bar"}`,
					false,
				},

				// Equal label selector in the expression.
				{
					`job=""`,
					``,
					true,
				},
				{
					`job="foo"`,
					`up{job!~"foo|bar",job="foo"}`,
					false,
				},
				{
					`job="bar"`,
					`up{job!~"foo|bar",job="bar"}`,
					false,
				},
				{
					`job="fred"`,
					`up{job!~"foo|bar",job="fred"}`,
					false,
				},

				// Not equal label selector in the expression.
				{
					`job!=""`,
					`up{job!="",job!~"foo|bar"}`,
					false,
				},
				{
					`job!="foo"`,
					`up{job!="foo",job!~"foo|bar"}`,
					false,
				},
				{
					`job!="bar"`,
					`up{job!="bar",job!~"foo|bar"}`,
					false,
				},
				{
					`job!="fred"`,
					`up{job!="fred",job!~"foo|bar"}`,
					false,
				},

				// Regexp label selector in the expression.
				{
					`job=~""`,
					``,
					true,
				},
				{
					// up{job!~"foo|bar",job=~"foo"} would return no result.
					`job=~"foo"`,
					``,
					true,
				},
				{
					// up{job!~"foo|bar",job=~"bar"} would return no result.
					`job=~"bar"`,
					``,
					true,
				},
				{
					`job=~"fred"`,
					`up{job!~"foo|bar",job=~"fred"}`,
					false,
				},
				{
					`job=~"foo|fred"`,
					`up{job!~"foo|bar",job=~"foo|fred"}`,
					false,
				},
				{
					`job=~"foo|bar"`,
					``,
					true,
				},

				// Not-regexp label selector in the expression.
				{
					`job!~""`,
					`up{job!~"",job!~"foo|bar"}`,
					false,
				},
				{
					`job!~"foo"`,
					`up{job!~"foo",job!~"foo|bar"}`,
					false,
				},
				{
					`job!~"bar"`,
					`up{job!~"bar",job!~"foo|bar"}`,
					false,
				},
				{
					`job!~"fred"`,
					`up{job!~"foo|bar",job!~"fred"}`,
					false,
				},
				{
					`job!~"foo|fred"`,
					`up{job!~"foo|bar",job!~"foo|fred"}`,
					false,
				},
				{
					`job!~"foo|bar"`,
					`up{job!~"foo|bar"}`,
					false,
				},
			},
		},
	} {
		t.Run(fmt.Sprintf("enforcer=%q", tc.enforcedMatcher.String()), func(t *testing.T) {
			enforcer := NewPromQLEnforcer(true, tc.enforcedMatcher)

			for _, stc := range tc.stcs {
				expr := fmt.Sprintf("up{%s}", stc.labelSelector)
				t.Run(expr, func(t *testing.T) {
					got, err := enforcer.Enforce(expr)
					if stc.err {
						if err == nil {
							t.Fatalf("expected error, got nil")
						}

						if !errors.Is(err, ErrIllegalLabelMatcher) {
							t.Fatalf("expected err,ErrIllegalLabelMatcher error, got %s", err)
						}

						return
					}

					if err != nil {
						t.Fatalf("expected no error, got %s", err.Error())
					}

					if got != stc.exp {
						t.Fatalf("expected expression %q, got %q", stc.exp, got)
					}
				})
			}
		})
	}
}
