module github.com/prometheus-community/prom-label-proxy

go 1.13

require (
	github.com/efficientgo/tools/core v0.0.0-20210201224146-3d78f4d30648
	github.com/go-openapi/runtime v0.19.28
	github.com/go-openapi/strfmt v0.20.1
	github.com/pkg/errors v0.9.1
	github.com/prometheus/alertmanager v0.22.2
	github.com/prometheus/prometheus v1.8.2-0.20210621150501-ff58416a0b02
)

replace github.com/prometheus/alertmanager => github.com/prometheus/alertmanager v0.22.1-0.20210623090652-e3fb99cc2d24
