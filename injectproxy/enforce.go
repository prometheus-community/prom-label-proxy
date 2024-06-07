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

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
)

// PromQLEnforcer can enforce label matchers in PromQL expressions.
type PromQLEnforcer struct {
	labelMatchers  map[string]*labels.Matcher
	errorOnReplace bool
}

func NewPromQLEnforcer(errorOnReplace bool, ms ...*labels.Matcher) *PromQLEnforcer {
	entries := make(map[string]*labels.Matcher)

	for _, matcher := range ms {
		entries[matcher.Name] = matcher
	}

	return &PromQLEnforcer{
		labelMatchers:  entries,
		errorOnReplace: errorOnReplace,
	}
}

var (
	// ErrQueryParse is returned when the input query is invalid.
	ErrQueryParse = errors.New("failed to parse query string")

	// ErrIllegalLabelMatcher is returned when the input query contains a conflicting label matcher.
	ErrIllegalLabelMatcher = errors.New("conflicting label matcher")

	// ErrEnforceLabel is returned when the label matchers couldn't be enforced.
	ErrEnforceLabel = errors.New("failed to enforce label")
)

// Enforce the label matchers in a PromQL expression.
func (ms *PromQLEnforcer) Enforce(q string) (string, error) {
	expr, err := parser.ParseExpr(q)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrQueryParse, err)
	}

	if err := ms.EnforceNode(expr); err != nil {
		if errors.Is(err, ErrIllegalLabelMatcher) {
			return "", err
		}

		return "", fmt.Errorf("%w: %w", ErrEnforceLabel, err)
	}

	return expr.String(), nil
}

// EnforceNode walks the given node recursively
// and enforces the given label enforcer on it.
//
// Whenever a parser.MatrixSelector or parser.VectorSelector AST node is found,
// their label enforcer is being potentially modified.
// If a node's label matcher has the same name as a label matcher
// of the given enforcer, then it will be replaced.
func (ms PromQLEnforcer) EnforceNode(node parser.Node) error {
	switch n := node.(type) {
	case *parser.EvalStmt:
		if err := ms.EnforceNode(n.Expr); err != nil {
			return err
		}

	case parser.Expressions:
		for _, e := range n {
			if err := ms.EnforceNode(e); err != nil {
				return err
			}
		}

	case *parser.AggregateExpr:
		if err := ms.EnforceNode(n.Expr); err != nil {
			return err
		}

	case *parser.BinaryExpr:
		if err := ms.EnforceNode(n.LHS); err != nil {
			return err
		}

		if err := ms.EnforceNode(n.RHS); err != nil {
			return err
		}

	case *parser.Call:
		if err := ms.EnforceNode(n.Args); err != nil {
			return err
		}

	case *parser.SubqueryExpr:
		if err := ms.EnforceNode(n.Expr); err != nil {
			return err
		}

	case *parser.ParenExpr:
		if err := ms.EnforceNode(n.Expr); err != nil {
			return err
		}

	case *parser.UnaryExpr:
		if err := ms.EnforceNode(n.Expr); err != nil {
			return err
		}

	case *parser.NumberLiteral, *parser.StringLiteral:
	// nothing to do

	case *parser.MatrixSelector:
		// inject labelselector
		if vs, ok := n.VectorSelector.(*parser.VectorSelector); ok {
			var err error
			vs.LabelMatchers, err = ms.EnforceMatchers(vs.LabelMatchers)
			if err != nil {
				return err
			}
		}

	case *parser.VectorSelector:
		// inject labelselector
		var err error
		n.LabelMatchers, err = ms.EnforceMatchers(n.LabelMatchers)
		if err != nil {
			return err
		}

	default:
		panic(fmt.Errorf("parser.Walk: unhandled node type %T", n))
	}

	return nil
}

// EnforceMatchers appends the configured label matcher if not present.
// If the label matcher that is to be injected is present (by labelname) but
// different (either by match type or value) the behavior depends on the
// errorOnReplace variable and the enforced matcher(s):
// * if errorOnReplace is true, an error is returned,
// * if errorOnReplace is false and the label matcher type is '=', the existing matcher is silently replaced.
// * otherwise the existing matcher is preserved.
func (ms PromQLEnforcer) EnforceMatchers(targets []*labels.Matcher) ([]*labels.Matcher, error) {
	var res []*labels.Matcher

	for _, target := range targets {
		if matcher, ok := ms.labelMatchers[target.Name]; ok {
			// matcher.String() returns something like "labelfoo=value"
			if ms.errorOnReplace && matcher.String() != target.String() {
				return res, fmt.Errorf("%w: label matcher value %q conflicts with injected value %q", ErrIllegalLabelMatcher, matcher.String(), target.String())
			}

			// Drop the existing matcher only if the enforced matcher is an
			// equal matcher.
			if matcher.Type == labels.MatchEqual {
				continue
			}
		}

		res = append(res, target)
	}

	for _, enforcedMatcher := range ms.labelMatchers {
		res = append(res, enforcedMatcher)
	}

	return res, nil
}
