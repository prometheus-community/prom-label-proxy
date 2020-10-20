# Demo: Using prom-label-proxy in front of demo Prometheus server.

1. Run prom-label-proxy with passthrough option:

```
prom-label-proxy -label job -upstream http://demo.robustperception.io:9090 -insecure-listen-address 127.0.0.1:8080 -non-api-path-passthrough
```

2. In separate terminal run caddy that injects job=prometheus when accessing localhost:8082 and injecting job=pushgateway on localhost:8081:

```
docker run -it --rm --net=host -v $PWD/Caddyfile:/etc/caddy/Caddyfile caddy
```

3. Access `localhost:8082` and compare with original `http://demo.robustperception.io:9090` server. You should be able to access only `job=pushgateway`
or `job=prometheus` data depending on the port.
