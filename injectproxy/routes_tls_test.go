// Copyright The Prometheus Authors
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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeTestCertKey generates a self-signed certificate and key and writes them
// to temporary PEM files, returning their paths.
func writeTestCertKey(t *testing.T) (certFile, keyFile string) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}
	keyBytes, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("failed to marshal key: %v", err)
	}

	dir := t.TempDir()
	certFile = filepath.Join(dir, "cert.pem")
	keyFile = filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatalf("failed to write cert: %v", err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes}), 0o600); err != nil {
		t.Fatalf("failed to write key: %v", err)
	}
	return certFile, keyFile
}

func transportTLSConfig(t *testing.T, r *routes) *tls.Config {
	t.Helper()
	proxy, ok := r.handler.(*httputil.ReverseProxy)
	if !ok {
		t.Fatalf("handler is not a *httputil.ReverseProxy, got %T", r.handler)
	}
	transport, ok := proxy.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport is not a *http.Transport, got %T", proxy.Transport)
	}
	return transport.TLSClientConfig
}

func TestNewRoutesUpstreamTLSConfig(t *testing.T) {
	certFile, keyFile := writeTestCertKey(t)
	upstream, _ := url.Parse("https://upstream.example")

	r, err := NewRoutes(upstream, proxyLabel, HTTPFormEnforcer{ParameterName: proxyLabel},
		WithUpstreamClientCert(certFile, keyFile),
		WithUpstreamServerName("custom.example"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg := transportTLSConfig(t, r)
	if cfg.ServerName != "custom.example" {
		t.Errorf("ServerName = %q, want %q", cfg.ServerName, "custom.example")
	}
	if got := len(cfg.Certificates); got != 1 {
		t.Errorf("len(Certificates) = %d, want 1", got)
	}
}

func TestNewRoutesUpstreamClientCertErrors(t *testing.T) {
	certFile, keyFile := writeTestCertKey(t)
	upstream, _ := url.Parse("https://upstream.example")

	for name, opt := range map[string]Option{
		"only cert provided": WithUpstreamClientCert(certFile, ""),
		"only key provided":  WithUpstreamClientCert("", keyFile),
		"nonexistent files":  WithUpstreamClientCert("/does/not/exist.crt", "/does/not/exist.key"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewRoutes(upstream, proxyLabel, HTTPFormEnforcer{ParameterName: proxyLabel}, opt); err == nil {
				t.Error("expected an error, got nil")
			}
		})
	}
}
