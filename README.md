[![Docker Repository on Quay](https://quay.io/repository/prometheuscommunity/prom-label-proxy/status "Docker Repository on Quay")](https://quay.io/repository/prometheuscommunity/prom-label-proxy)

# prom-label-proxy

The prom-label-proxy can enforce a given label in a given PromQL query, in Prometheus API responses or in Alertmanager API requests. As an example (but not only),
this allows read multi-tenancy for projects like Prometheus, Alertmanager or Thanos.

This proxy does not perform authentication or authorization, this has to happen before the request reaches this proxy, allowing you to use any authN/authZ system you want. The [kube-rbac-proxy](https://github.com/brancz/kube-rbac-proxy) is an example for such an additional building block. Additionally, you can use prom-label-proxy as a library in your own proxy, like what is done in [prom-authzed-proxy](https://github.com/authzed/prom-authzed-proxy).

### Risks outside the scope of this project

It's not a goal for this project to solve write tenant isolation for multi-tenant Prometheus:

* If a tenant controls its scrape target configuration the tenant can set arbitrary labels via its relabelling configuration, thereby being able to pollute other tenant's metrics.
* If the ingestion configuration [honor_labels](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#scrape_config) is set for a tenant's target, that target can pollute other tenant's metrics as Prometheus respects any labels exposed by the target.

See [Prometheus Operator](https://github.com/prometheus-operator/prometheus-operator) label enforcement, [Thanos soft/hard tenancy](https://thanos.io/tip/proposals/201812_thanos-remote-receive.md/#architecture) or [Cortex](https://cortexmetrics.io/) as example solution to that.

## Installing `prom-label-proxy`

### Helm

See: https://github.com/prometheus-community/helm-charts/tree/main/charts/prom-label-proxy

### Docker

We publish docker images for each release, see:

* [`quay.io/prometheuscommunity/prom-label-proxy`](https://quay.io/repository/prometheuscommunity/prom-label-proxy?tab=tags) for newest images
* `quay.io/coreos/prom-label-proxy:v0.1.0` for the initial v0.1.0 release.

### Building from source

If you want to build `prom-label-proxy` from source you would need a working installation of
the [Go](https://golang.org/) 1.15+ [toolchain](https://github.com/golang/tools) (`GOPATH`, `PATH=${GOPATH}/bin:${PATH}`).

`prom-label-proxy` can be downloaded and built by running:

```bash
go get github.com/prometheus-community/prom-label-proxy
```

## How does this project work?

This application proxies the following endpoints and it ensures that a particular label is enforced in the particular request and response:

* `/federate` for GET method (Prometheus)
* `/api/v1/query_exemplars` for GET and POST methods (Prometheus/Thanos)
* `/api/v1/query` for GET and POST methods (Prometheus/Thanos)
* `/api/v1/query_range` for GET and POST methods (Prometheus/Thanos)
* `/api/v1/series` for GET method (Prometheus/Thanos)
* `/api/v1/rules` for GET method (Prometheus/Thanos)
* `/api/v1/alerts` for GET method (Prometheus/Thanos)
* `/api/v2/silences` for GET and POST methods (Alertmanager)
* `/api/v2/silence/` for DELETE (Alertmanager)
* `/api/v2/alerts/groups` for GET (Alertmanager)
* `/api/v2/alerts` for GET (Alertmanager)

When started with the `-enable-label-apis` flag, the application can also proxy the following endpoints:

* `/api/v1/labels` for GET and POST methods (Prometheus/Thanos)
* `/api/v1/label/<name>/values` for GET method (Prometheus/Thanos)

You can run `prom-label-proxy` to enforce the value of the `tenant` label
provided in the client's request via the `tenant` HTTP query/form parameter:

```
prom-label-proxy \
   -query-param tenant \
   -label tenant \
   -upstream http://demo.do.prometheus.io:9090 \
   -insecure-listen-address 127.0.0.1:8080
```

Accessing the demo Prometheus APIs on `http://127.0.0.1:8080` will now expect
that the client's request provides the `tenant` label value using the `tenant`
HTTP query parameter:

```bash
➜  ~ curl http://127.0.0.1:8080/api/v1/query\?query="up"
{"error":"The \"tenant\" query parameter must be provided.","errorType":"prom-label-proxy","status":"error"}
➜  ~ curl http://127.0.0.1:8080/api/v1/query\?query="up"\&tenant\="something"
{"status":"success","data":{"resultType":"vector","result":[]}}%
```

You can provide multiple values for the label using several `tenant` HTTP query parameters:

```bash
➜  ~ curl http://127.0.0.1:8080/api/v1/query\?query="up"\&tenant\="something"\&tenant\="anything"
{"status":"success","data":{"resultType":"vector","result":[]}}%
```

It also works with POST requests:

```bash
➜  ~ curl http://127.0.0.1:8080/api/v1/query" -d "tenant=foo"
{"status":"success","data":{"resultType":"vector","result":[]}}%
```

Alternatively, `prom-label-proxy` can use a custom HTTP header instead HTTP parameters:

```
prom-label-proxy \
   -header-name X-Tenant \
   -label tenant \
   -upstream http://demo.do.prometheus.io:9090 \
   -insecure-listen-address 127.0.0.1:8080
```

```bash
➜  ~ curl -H 'X-Tenant: something' http://127.0.0.1:8080/api/v1/query\?query="up"
{"status":"success","data":{"resultType":"vector","result":[]}}%
```

You can provide multiple values for the label using several HTTP headers:

```bash
➜  ~ curl -H 'X-Tenant=something' -H 'X-Tenant=anything' http://127.0.0.1:8080/api/v1/query\?query="up"
{"status":"success","data":{"resultType":"vector","result":[]}}%
```

A last option is to provide a static value for the label:

```
prom-label-proxy \
   -label tenant \
   -label-value prometheus \
   -upstream http://demo.do.prometheus.io:9090 \
   -insecure-listen-address 127.0.0.1:8080
```

Now prom-label-proxy enforces the `tenant="prometheus"` label in all requests.

You can provide multiple static values for a label. For example:

```
prom-label-proxy \
   -label tenant \
   -label-value prometheus \
   -label-value alertmanager \
   -upstream http://demo.do.prometheus.io:9090 \
   -insecure-listen-address 127.0.0.1:8080
```

`prom-label-proxy` will enforce the `tenant=~"prometheus|alertmanager"` label selector in all requests.

You can match the label value using a regular expression with the `-regex-match` option. For example:

```
prom-label-proxy \
   -label-value '^foo-.+$' \
   -label namespace \
   -upstream http://demo.do.prometheus.io:9090 \
   -insecure-listen-address 127.0.0.1:8080 \
   -regex-match
```

> :warning: The above feature is experimental. Be careful when using this option, it may expose sensitive metrics if you use a too permissive expression.

To error out when the query already contains a label matcher that conflicts with the one the proxy would inject, you can use the `-error-on-replace` option. For example:

```
prom-label-proxy \
   -header-name X-Namespace \
   -label namespace \
   -upstream http://demo.do.prometheus.io:9090 \
   -insecure-listen-address 127.0.0.1:8080 \
   -error-on-replace
```

Once again for clarity: **this project only enforces a particular label in the respective calls to Prometheus, it in itself does not authenticate or
authorize the requesting entity in any way, this has to be built around this project.**

### Federate endpoint

The proxy ensures that all selectors passed as matchers to the `/federate` endpoint _must_ contain that exact match of the particular label (and throws away all other matchers for the label).

### Query endpoints

For the two query endpoints (`/api/v1/query` and `/api/v1/query_range`), the proxy parses the PromQL expression and modifies all selectors in the same way. The label-key is configured as a flag on the binary and the label-value is passed as a query parameter.

For example, if requesting the PromQL query

```
http_requests_total{namespace=~"a.*"}
```

and specifying the namespace label must be enforced to `b`, then the query will be re-written to


```
http_requests_total{namespace=~"b"}
```

This is enforced for any case, whether a label matcher is specified in the original query or not.

### Metadata endpoints

Similar to query endpoint, for metadata endpoints `/api/v1/series`, `/api/v1/labels`, `/api/v1/label/<name>/values` the proxy injects the specified label all the provided `match[]` selectors.

NOTE: When the `/api/v1/labels` and `/api/v1/label/<name>/values` endpoints were added to `prom-label-proxy`, the Prometheus and Thanos endpoints didn't support the `match[]` parameter hence the `prom-label-proxy` labels endpoints are disabled by default. Use the `-enable-label-apis` flag to enable with care. Ensure that the upstream endpoints support label selectors:
* Prometheus >= [2.24.0](https://github.com/prometheus/prometheus/releases/tag/v2.24.0)
* Thanos >= [v0.18.0](https://github.com/thanos-io/thanos/releases/tag/v0.18.0) at least, >= [0.23.0](https://github.com/thanos-io/thanos/releases/tag/v0.23.0) recommended for better performances.

### Rules endpoint

The proxy requests the `/api/v1/rules` Prometheus endpoint, discards the rules that don't contain an exact match of the label(s) and returns the modified response to the client.

### Alerts endpoint

The proxy requests the `/api/v1/alerts` Prometheus endpoint, discards the rules that don't contain an exact match of the label(s) and returns the modified response to the client.

### Silences endpoint

The proxy ensures the following:

* `GET` requests to the `/api/v2/silences` endpoint contain a `filter` parameter that matches exactly the particular label and throws away all other matchers for the label.
* `POST` requests to the `/api/v2/silences` endpoint can only affect silences that match the label and the label matcher is enforced.
* `DELETE` requests to the `/api/v2/silence/` endpoint can only affect silences that match the label.

:rotating_light: `prom-label-proxy` doesn't support multiple label values for the Silences endpoints :rotating_light:

## Example use

The concrete setup being shipped in OpenShift starting with 4.0: the proxy is configured to work with the label-key: namespace. In order to ensure that this is secure is it paired with the [kube-rbac-proxy](https://github.com/brancz/kube-rbac-proxy) and its URL rewrite functionality, meaning first ServiceAccount token authentication is performed, and then the kube-rbac-proxy authorization to see whether the requesting entity is allowed to retrieve the metrics for the requested namespace. The RBAC role we chose to authorize against is the same as the Kubernetes Resource Metrics API, the reasoning being, if an entity can `kubectl top pod` in a namespace, it can see cAdvisor metrics (container_memory_rss, container_cpu_usage_seconds_total, etc.).
