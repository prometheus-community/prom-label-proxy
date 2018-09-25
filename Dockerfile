FROM openshift/origin-base AS build

ENV GOPATH /go
RUN mkdir $GOPATH
RUN yum install -y golang make

COPY . $GOPATH/src/github.com/openshift/prom-label-proxy
RUN cd $GOPATH/src/github.com/openshift/prom-label-proxy && make build

FROM openshift/origin-base
COPY --from=build /go/src/github.com/openshift/prom-label-proxy/_output/linux/amd64/prom-label-proxy /usr/bin/prom-kube-proxy

LABEL io.k8s.display-name="" \
      io.k8s.description="" \
      io.openshift.tags="prometheus" \
      maintainer="Frederic Branczyk <fbranczy@redhat.com>"

# doesn't require a root user.
USER 1001
ENTRYPOINT ["/usr/bin/prom-kube-proxy"]
