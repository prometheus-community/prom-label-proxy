module github.com/openshift/prom-label-proxy

go 1.13

// v2.18.1
replace github.com/prometheus/prometheus => github.com/prometheus/prometheus v0.0.0-20200507164740-ecee9c8abfd1

require (
	github.com/go-openapi/runtime v0.19.15
	github.com/go-openapi/strfmt v0.19.5
	github.com/pkg/errors v0.9.1
	github.com/prometheus/alertmanager v0.20.0
	github.com/prometheus/prometheus v1.8.2-0.20200106144642-d9613e5c466c
)
