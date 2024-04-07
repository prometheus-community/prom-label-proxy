// Copyright 2024 Cloudera Inc
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

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/prometheus-community/prom-label-proxy/injectproxy"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
)

func main() {
	// Read PromQL query from standard input
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter PromQL query: ")
	query, err := reader.ReadString('\n')
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
		os.Exit(1)
	}

	// Remove newline character from the end of the input
	query = strings.TrimSpace(query)

	// Parse the query into an expression (AST)
	expr, err := parser.ParseExpr(query)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing PromQL: %v\n", err)
		os.Exit(1)
	}

	// Create an expression enforcer to
	// Match label "foo" to value "bar"
	matcher := &labels.Matcher{
		Name:  "foo",
		Type:  labels.MatchEqual,
		Value: "bar",
	}
	// Create an instance of the Enforcer that replaces (see "false" value)
	// label values
	enf := injectproxy.NewEnforcer(false, matcher)

	// Modify PromQL query
	if err := enf.EnforceNode(expr); err != nil {
		fmt.Fprintf(os.Stderr, "Error modifying PromQL: %v\n", err)
		os.Exit(1)
	}

	// Use the potentially modified expression to render a
	// modified PromQL query
	modifiedQuery := expr.String()

	// Use the expression to output a modified PromQL query
	fmt.Println(modifiedQuery)
}
