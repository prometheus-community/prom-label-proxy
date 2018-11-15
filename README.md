# prom-label-proxy

The prom-label-proxy enforces a given label in a given PromQL proxy.

This proxy does not perform authentication or authorization, this has to happen
before the request reaches this proxy. The
[kube-rbac-proxy](https://github.com/brancz/kube-rbac-proxy) is an example for
such an additional building block.


Risks outside the scope of this project:

- If a tenant controls its scrape target configuration the tenant can set
  arbitrary labels via its relabelling configuration, thereby being able to
  pollute other tenant's metrics.

- If the ingestion configuration
  [honor_labels](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#scrape_config)
  is set for a tenant's target, that target can pollute other tenant's metrics
  as Prometheus respects any labels exposed by the target.
