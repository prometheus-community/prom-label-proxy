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

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"syscall"

	"github.com/metalmatze/signal/internalserver"
	"github.com/oklog/run"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/prometheus/promql/parser"

	"github.com/prometheus-community/prom-label-proxy/injectproxy"
)

type arrayFlags []string

// String is the method to format the flag's value, part of the flag.Value interface.
// The String method's output will be used in diagnostics.
func (i *arrayFlags) String() string {
	return fmt.Sprint(*i)
}

// Set is the method to set the flag value, part of the flag.Value interface.
func (i *arrayFlags) Set(value string) error {
	if value == "" {
		return nil
	}

	*i = append(*i, value)
	return nil
}

func main() {
	var (
		insecureListenAddress           string
		internalListenAddress           string
		upstream                        string
		queryParam                      string
		headerName                      string
		label                           string
		labelValues                     arrayFlags
		enableLabelAPIs                 bool
		unsafePassthroughPaths          string // Comma-delimited string.
		insecureSkipVerify              bool
		errorOnReplace                  bool
		regexMatch                      bool
		headerUsesListSyntax            bool
		rulesWithActiveAlerts           bool
		labelMatchersForRulesAPI        bool
		promQLDurationExpressionParsing bool
		promQLExperimentalFunctions     bool
	)

	flagset := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	flagset.StringVar(&insecureListenAddress, "insecure-listen-address", "", "The address the prom-label-proxy HTTP server should listen on.")
	flagset.StringVar(&internalListenAddress, "internal-listen-address", "", "The address the internal prom-label-proxy HTTP server should listen on to expose metrics about itself.")
	flagset.StringVar(&queryParam, "query-param", "", "Name of the HTTP parameter that contains the tenant value.At most one of -query-param, -header-name and -label-value should be given. If the flag isn't defined and neither -header-name nor -label-value is set, it will default to the value of the -label flag.")
	flagset.StringVar(&headerName, "header-name", "", "Name of the HTTP header name that contains the tenant value. At most one of -query-param, -header-name and -label-value should be given.")
	flagset.StringVar(&upstream, "upstream", "", "The upstream URL to proxy to.")
	flagset.StringVar(&label, "label", "", "The label name to enforce in all proxied PromQL queries.")
	flagset.Var(&labelValues, "label-value", "A fixed label value to enforce in all proxied PromQL queries. At most one of -query-param, -header-name and -label-value should be given. It can be repeated in which case the proxy will enforce the union of values.")
	flagset.BoolVar(&enableLabelAPIs, "enable-label-apis", false, "When specified proxy allows to inject label to label APIs like /api/v1/labels and /api/v1/label/<name>/values. "+
		"NOTE: Enable with care because filtering by matcher is not implemented in older versions of Prometheus (>= v2.24.0 required) and Thanos (>= v0.18.0 required, >= v0.23.0 recommended). If enabled and "+
		"any labels endpoint does not support selectors, the injected matcher will have no effect.")
	flagset.StringVar(&unsafePassthroughPaths, "unsafe-passthrough-paths", "", "Comma delimited allow list of exact HTTP path segments that should be allowed to hit upstream URL without any enforcement. "+
		"This option is checked after Prometheus APIs, you cannot override enforced API endpoints to be not enforced with this option. Use carefully as it can easily cause a data leak if the provided path is an important "+
		"API (like /api/v1/configuration) which isn't enforced by prom-label-proxy. NOTE: \"all\" matching paths like \"/\" or \"\" and regex are not allowed.")
	flagset.BoolVar(&insecureSkipVerify, "insecure-skip-verify", false, "When specified, the proxy will bypass validation of the server's TLS/SSL certificate.")
	flagset.BoolVar(&errorOnReplace, "error-on-replace", false, "When specified, the proxy will return HTTP status code 400 if the query already contains a label matcher that differs from the one the proxy would inject.")
	flagset.BoolVar(&regexMatch, "regex-match", false, "When specified, the tenant name is treated as a regular expression. In this case, only one tenant name should be provided.")
	flagset.BoolVar(&headerUsesListSyntax, "header-uses-list-syntax", false, "When specified, the header line value will be parsed as a comma-separated list. This allows a single tenant header line to specify multiple tenant names.")
	flagset.BoolVar(&rulesWithActiveAlerts, "rules-with-active-alerts", false, "When true, the proxy will return alerting rules with active alerts matching the tenant label even when the tenant label isn't present in the rule's labels.")
	flagset.BoolVar(&labelMatchersForRulesAPI, "enable-label-matchers-for-rules-api", false, "When true, the proxy uses label matchers when querying the /api/v1/rules endpoint. NOTE: Enable with care because filtering by label matcher is not implemented in older versions of Prometheus (>= 2.54.0 required) and Thanos (>= v0.25.0 required). If not implemented by upstream, the response will not be filtered accordingly.")
	flagset.BoolVar(&promQLDurationExpressionParsing, "enable-promql-duration-expression-parsing", false, "When true, the proxy supports arithmetic for durations in PromQL expressions.")
	flagset.BoolVar(&promQLExperimentalFunctions, "enable-promql-experimental-functions", false, "When true, the proxy supports experimental functions in PromQL expressions.")

	//nolint: errcheck // Parse() will exit on error.
	flagset.Parse(os.Args[1:])
	if label == "" {
		log.Fatalf("-label flag cannot be empty")
	}

	if len(labelValues) == 0 && queryParam == "" && headerName == "" {
		queryParam = label
	}

	if len(labelValues) > 0 {
		if queryParam != "" || headerName != "" {
			log.Fatalf("at most one of -query-param, -header-name and -label-value must be set")
		}
	} else if queryParam != "" && headerName != "" {
		log.Fatalf("at most one of -query-param, -header-name and -label-value must be set")
	}

	upstreamURL, err := url.Parse(upstream)
	if err != nil {
		log.Fatalf("Failed to build parse upstream URL: %v", err)
	}

	if upstreamURL.Scheme != "http" && upstreamURL.Scheme != "https" {
		log.Fatalf("Invalid scheme for upstream URL %q, only 'http' and 'https' are supported", upstream)
	}

	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	opts := []injectproxy.Option{injectproxy.WithPrometheusRegistry(reg)}
	if enableLabelAPIs {
		opts = append(opts, injectproxy.WithEnabledLabelsAPI())
	}

	if len(unsafePassthroughPaths) > 0 {
		opts = append(opts, injectproxy.WithPassthroughPaths(strings.Split(unsafePassthroughPaths, ",")))
	}

	if insecureSkipVerify {
		opts = append(opts, injectproxy.WithInsecureSkipVerify())
	}

	if errorOnReplace {
		opts = append(opts, injectproxy.WithErrorOnReplace())
	}

	if rulesWithActiveAlerts {
		opts = append(opts, injectproxy.WithActiveAlerts())
	}

	if labelMatchersForRulesAPI {
		opts = append(opts, injectproxy.WithLabelMatchersForRulesAPI())
	}

	if regexMatch {
		if len(labelValues) > 0 {
			if len(labelValues) > 1 {
				log.Fatalf("Regex match is limited to one label value")
			}

			compiledRegex, err := regexp.Compile(labelValues[0])
			if err != nil {
				log.Fatalf("Invalid regexp: %v", err.Error())
				return
			}

			if compiledRegex.MatchString("") {
				log.Fatalf("Regex should not match empty string")
				return
			}
		}

		opts = append(opts, injectproxy.WithRegexMatch())
	}

	var extractLabeler injectproxy.ExtractLabeler
	switch {
	case len(labelValues) > 0:
		extractLabeler = injectproxy.StaticLabelEnforcer(labelValues)
	case queryParam != "":
		extractLabeler = injectproxy.HTTPFormEnforcer{ParameterName: queryParam}
	case headerName != "":
		extractLabeler = injectproxy.HTTPHeaderEnforcer{Name: http.CanonicalHeaderKey(headerName), ParseListSyntax: headerUsesListSyntax}
	}

	parser.ExperimentalDurationExpr = promQLDurationExpressionParsing
	parser.EnableExperimentalFunctions = promQLExperimentalFunctions

	var g run.Group
	{
		// Run the insecure HTTP server.
		routes, err := injectproxy.NewRoutes(upstreamURL, label, extractLabeler, opts...)
		if err != nil {
			log.Fatalf("Failed to create injectproxy Routes: %v", err)
		}

		mux := http.NewServeMux()
		mux.Handle("/", routes)

		l, err := net.Listen("tcp", insecureListenAddress)
		if err != nil {
			log.Fatalf("Failed to listen on insecure address: %v", err)
		}

		srv := &http.Server{Handler: mux}

		g.Add(func() error {
			log.Printf("Listening insecurely on %v", l.Addr())
			if err := srv.Serve(l); err != nil && err != http.ErrServerClosed {
				log.Printf("Server stopped with %v", err)
				return err
			}
			return nil
		}, func(error) {
			srv.Close()
		})
	}

	if internalListenAddress != "" {
		// Run the internal HTTP server.
		h := internalserver.NewHandler(
			internalserver.WithName("Internal prom-label-proxy API"),
			internalserver.WithPrometheusRegistry(reg),
			internalserver.WithPProf(),
		)
		// Run the HTTP server.
		l, err := net.Listen("tcp", internalListenAddress)
		if err != nil {
			log.Fatalf("Failed to listen on internal address: %v", err)
		}

		srv := &http.Server{Handler: h}

		g.Add(func() error {
			log.Printf("Listening on %v for metrics and pprof", l.Addr())
			if err := srv.Serve(l); err != nil && err != http.ErrServerClosed {
				log.Printf("Internal server stopped with %v", err)
				return err
			}
			return nil
		}, func(error) {
			srv.Close()
		})
	}

	g.Add(run.SignalHandler(context.Background(), syscall.SIGINT, syscall.SIGTERM))

	if err := g.Run(); err != nil {
		if !errors.As(err, &run.SignalError{}) {
			log.Printf("Server stopped with %v", err)
			os.Exit(1)
		}
		log.Print("Caught signal; exiting gracefully...")
	}
}
