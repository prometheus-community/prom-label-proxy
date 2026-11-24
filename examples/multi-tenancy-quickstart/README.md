# Quickstart Demonstration: Read Multi-Tenancy

This is an end-to-end example demonstrating how 'prom-label-proxy' enables read multi-tenancy by intercepting Prometheus API requests and injecting a tenant-specific label selector into PromQL queries.
To keep the setup minimal and self-contained, it uses Prometheus's `relabel_configs` feature to generate mock metrics which are pre-labeled for a specific tenant (`tenant-a`). 
This eliminates the need for deploying external metric exporters.

## Prerequisites

* Docker and Docker Compose installed.
* `curl` and `jq` installed for testing queries.

## Flow Description

1. **Prometheus** is configured to scrape itself and apply a relabeling rule that injects the label `tenant="tenant-a"` to all ingested metrics.
2. **prom-label-proxy** listens locally on port `8080` and acts as a gateway to **Prometheus**.
3. When a client requests metrics via the proxy, they must provide the `X-Tenant` HTTP header, as per arguments passed to **prom-label-proxy** via compose setup.
4. The proxy parses the PromQL query, rewrites it to include `{tenant="<X-Tenant-header-value>"}`, and forwards it to the upstream **Prometheus**.

## Step-by-Step

### 1. Start the Environment

Run the following command to start both containers in detached mode:

```bash
docker compose up -d
```

*Note: Since the scrape interval is configured to `1s`, please wait 2–3 seconds to ensure Prometheus performs its initial scrape.*

### 2. Direct Prometheus Query (Bypassing the Proxy)

Query the upstream Prometheus instance directly on local port `9090` to verify that the raw data exists in the TSDB database and has the mock tenant label and value (`"tenant": "tenant-a"`):

```bash
curl -s "http://localhost:9090/api/v1/query?query=up" | jq
```

The result is an existing Prometheus time series with our mock label:

```json
{
  "status": "success",
  "data": {
    "resultType": "vector",
    "result": [
      {
        "metric": {
          "__name__": "up",
          "instance": "localhost:9090",
          "job": "mock-tenant-data",
          "tenant": "tenant-a"
        },
        "value": [
          1789979542.480,
          "1"
        ]
      }
    ]
  }
}
```

### 3. Proxy Query Without HTTP Header

Attempt to query the proxy on port `8080` without passing any identification headers:

```bash
curl -s "http://localhost:8080/api/v1/query?query=up" | jq
```

The result is a Proxy error response indicating lack of required header value and a `400 Bad Request` status:

```json
{
  "error": "Missing HTTP header \"X-Tenant\".",
  "errorType": "prom-label-proxy",
  "status": "error"
}
```

### 4. Proxy Query As 'Authorized' Tenant (`tenant-a`)

Query the proxy while identifying (via the correct HTTP Header) as `tenant-a`. 
This means the proxy will automatically modify our query from `up` to `up{tenant="tenant-a"}`:

```bash
curl -s -H "X-Tenant: tenant-a" "http://localhost:8080/api/v1/query?query=up" | jq
```

The result is a `200 OK` response along with the metric data, because the injected label selector matches the data stored in the database:

```json
{
  "status": "success",
  "data": {
    "resultType": "vector",
    "result": [
      {
        "metric": {
          "__name__": "up",
          "instance": "localhost:9090",
          "job": "mock-tenant-data",
          "tenant": "tenant-a"
        },
        "value": [
          1789981109.441,
          "1"
        ]
      }
    ]
  }
}
```

### 5. Proxy Query As 'Unauthorized' Tenant (`tenant-b`)

Query the proxy while identifying as an 'unauthorized' `tenant-b`, meaning the final query will become `up{tenant="tenant-b"}`:

```bash
curl -s -H "X-Tenant: tenant-b" "http://localhost:8080/api/v1/query?query=up" | jq
```

While the request succeeds because the required `X-Tenant` header has been provided, the result is an empty array.
Tenant `tenant-b` does not match `tenant` label value set on any of the existing metrics stored in Prometheus.

```json
{
  "status": "success",
  "data": {
    "resultType": "vector",
    "result": []
  }
}
```

## Cleanup

To stop the containers and remove the resources created by this example, simply run:

```bash
docker compose down
```

## Disclaimer

This quickstart uses Prometheus `relabel_configs` strictly for mocking data visibility and providing a self-contained example of a full read-isolation flow. 
As per the project's core documentation, `prom-label-proxy` does not solve write tenant isolation for a multi-tenant Prometheus setup.