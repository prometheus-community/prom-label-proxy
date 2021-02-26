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
	"flag"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/prometheus-community/prom-label-proxy/injectproxy"
)

func main() {
	var (
		insecureListenAddress  string
		upstream               string
		label                  string
		enableLabelAPIs        bool
		unsafePassthroughPaths string // Comma-delimited string.
	)

	flagset := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	flagset.StringVar(&insecureListenAddress, "insecure-listen-address", "", "The address the prom-label-proxy HTTP server should listen on.")
	flagset.StringVar(&upstream, "upstream", "", "The upstream URL to proxy to.")
	flagset.StringVar(&label, "label", "", "The label to enforce in all proxied PromQL queries. "+
		"This label will be also required as the URL parameter to get the value to be injected. For example: -label=tenant will"+
		" make it required for this proxy to have URL in form of: <URL>?tenant=abc&other_params...")
	flagset.BoolVar(&enableLabelAPIs, "enable-label-apis", false, "When specified proxy allows to inject label to label APIs like /api/v1/labels and /api/v1/label/<name>/values."+
		"NOTE: Enable with care. Selection of matcher is still in development, see https://github.com/thanos-io/thanos/issues/3351 and https://github.com/prometheus/prometheus/issues/6178. If enabled and"+
		"any labels endpoint does not support selectors, injected matcher will be silently dropped.")
	flagset.StringVar(&unsafePassthroughPaths, "unsafe-passthrough-paths", "", "Comma delimited allow list of exact HTTP path segments should be allowed to hit upstream URL without any enforcement."+
		"This option is checked after Prometheus APIs, you can cannot override enforced API to be not enforced with this option. Use carefully as it can easily cause a data leak if the provided path is an important"+
		"API like targets or configuration. NOTE: \"all\" matching paths like \"/\" or \"\" and regex are not allowed.")

	//nolint: errcheck // Parse() will exit on error.
	flagset.Parse(os.Args[1:])
	if label == "" {
		log.Fatalf("-label flag cannot be empty")
	}

	upstreamURL, err := url.Parse(upstream)
	if err != nil {
		log.Fatalf("Failed to build parse upstream URL: %v", err)
	}

	if upstreamURL.Scheme != "http" && upstreamURL.Scheme != "https" {
		log.Fatalf("Invalid scheme for upstream URL %q, only 'http' and 'https' are supported", upstream)
	}

	var opts []injectproxy.Option
	if enableLabelAPIs {
		opts = append(opts, injectproxy.WithEnabledLabelsAPI())
	}
	if len(unsafePassthroughPaths) > 0 {
		opts = append(opts, injectproxy.WithPassthroughPaths(strings.Split(unsafePassthroughPaths, ",")))
	}
	routes, err := injectproxy.NewRoutes(upstreamURL, label, opts...)
	if err != nil {
		log.Fatalf("Failed to create injectproxy Routes: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", routes)

	srv := &http.Server{Handler: mux}

	l, err := net.Listen("tcp", insecureListenAddress)
	if err != nil {
		log.Fatalf("Failed to listen on insecure address: %v", err)
	}

	errCh := make(chan error)
	go func() {
		log.Printf("Listening insecurely on %v", l.Addr())
		errCh <- srv.Serve(l)
	}()

	term := make(chan os.Signal, 1)
	signal.Notify(term, os.Interrupt, syscall.SIGTERM)

	select {
	case <-term:
		log.Print("Received SIGTERM, exiting gracefully...")
		srv.Close()
	case err := <-errCh:
		if err != http.ErrServerClosed {
			log.Printf("Server stopped with %v", err)
		}
		os.Exit(1)
	}
}
