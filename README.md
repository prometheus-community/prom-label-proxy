# prom-label-proxy

The prom-label-proxy enforces a given label in a given PromQL proxy.

This proxy does not perform authentication or authorization, this has to happen before the request reaches this proxy. The [kube-rbac-proxy](https://github.com/brancz/kube-rbac-proxy) is an example for such an additional building block.

Risks outside the scope of this project:

- If a tenant controls its scrape target configuration the tenant can set arbitrary labels via its relabelling configuration, thereby being able to pollute other tenant's metrics.

- If the ingestion configuration [honor_labels](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#scrape_config) is set for a tenant's target, that target can pollute other tenant's metrics as Prometheus respects any labels exposed by the target.

## How does this project work?

What this proxy does is it proxies the `/federate`, `/api/v1/query`, `/api/v1/query_range` prometheus endpoints and ensures that a particular label is enforced in the particular query.

In the case of the federate endpoint, it ensures that all selectors passed as matchers to the federate endpoint _must_ contain that exact match of the particular label (and throws away all other matchers for the label). For the two query endpoints, it parses the PromQL expression and modifies all selectors in the same way. The label-key is configured as a flag on the binary and label-value is passed as a query parameter.

For example, if requesting the PromQL query

```
http_requests_total{namespace=~"a.*"}
```

and specifying the namespace label must be enforced to `b`, then the query will be re-written to


```
http_requests_total{namespace="b"}
```

This is enforced for any case, whether a label matcher is specified in the original query or not.

Once again for clarity: this project only enforces a particular label in the respective calls to Prometheus, it in itself does not authenticate or authorize the requesting entity in any way, this has to be built around this project.

## Example use

The concrete setup being shipped in OpenShift starting with 4.0: the proxy is configured to work with the label-key: namespace. In order to ensure that this is secure is it paired with the [kube-rbac-proxy](https://github.com/brancz/kube-rbac-proxy) and its URL rewrite functionality, meaning first ServiceAccount token authentication is performed, and then the kube-rbac-proxy authorization to see whether the requesting entity is allowed to retrieve the metrics for the requested namespace. The RBAC role we chose to authorize against is the same as the Kubernetes Resource Metrics API, the reasoning being, if an entity can `kubectl top pod` in a namespace, it can see cAdvisor metrics (container_memory_rss, container_cpu_usage_seconds_total, etc.).
