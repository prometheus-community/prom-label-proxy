module github.com/openshift/prom-label-proxy

go 1.12

require (
	github.com/go-openapi/runtime v0.19.4
	github.com/go-openapi/strfmt v0.19.2
	github.com/pkg/errors v0.8.1
	github.com/prometheus/alertmanager v0.20.0
	github.com/prometheus/prometheus v2.3.2+incompatible
)

replace github.com/prometheus/prometheus => github.com/prometheus/prometheus v0.0.0-20190818123050-43acd0e2e93f
