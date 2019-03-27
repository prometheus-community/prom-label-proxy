FROM openshift/origin-release:golang-1.10 AS builder

COPY . /go/src/github.com/openshift/prom-label-proxy
RUN cd /go/src/github.com/openshift/prom-label-proxy && make build; \
    mkdir -p /tmp/build; \
    cp /go/src/github.com/openshift/prom-label-proxy/_output/linux/$(go env GOARCH)/prom-label-proxy /tmp/build/prom-label-proxy

FROM openshift/origin-base
COPY --from=builder /tmp/build/prom-label-proxy /usr/bin/prom-kube-proxy

LABEL io.k8s.display-name="" \
      io.k8s.description="" \
      io.openshift.tags="prometheus" \
      maintainer="Frederic Branczyk <fbranczy@redhat.com>"

# doesn't require a root user.
USER 1001
ENTRYPOINT ["/usr/bin/prom-kube-proxy"]
