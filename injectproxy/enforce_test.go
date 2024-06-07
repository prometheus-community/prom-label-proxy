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
