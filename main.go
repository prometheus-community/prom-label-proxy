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
	"syscall"

	"github.com/prometheus-community/prom-label-proxy/injectproxy"
)

func main() {
	var (
		insecureListenAddress string
		upstream              string
		label                 string
	)

	flagset := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	flagset.StringVar(&insecureListenAddress, "insecure-listen-address", "", "The address the prom-label-proxy HTTP server should listen on.")
	flagset.StringVar(&upstream, "upstream", "", "The upstream URL to proxy to.")
	flagset.StringVar(&label, "label", "", "The label to enforce in all proxied PromQL queries.")
	//nolint: errcheck // Parse() will exit on error.
	flagset.Parse(os.Args[1:])

	upstreamURL, err := url.Parse(upstream)
	if err != nil {
		log.Fatalf("Failed to build parse upstream URL: %v", err)
	}

	routes := injectproxy.NewRoutes(upstreamURL, label)
	mux := http.NewServeMux()
	mux.Handle("/", routes)

	srv := &http.Server{Handler: mux}

	l, err := net.Listen("tcp", insecureListenAddress)
	if err != nil {
		log.Fatalf("Failed to listen on insecure address: %v", err)
	}

	errCh := make(chan error)
	go func() {
		log.Printf("Listening insecurely on %v", insecureListenAddress)
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
