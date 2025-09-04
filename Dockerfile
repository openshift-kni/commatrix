FROM registry.ci.openshift.org/ocp/4.20:oc-rpms AS oc
# Final image temporarly contains golang, will be removed after the e2e tests removed from our repo
FROM registry.ci.openshift.org/ocp/builder:rhel-9-golang-1.24-openshift-4.20

WORKDIR /go/src/github.com/openshift-kni/commatrix
RUN chmod -R a+w /go/src/github.com/openshift-kni/commatrix

COPY --from=oc /go/src/github.com/openshift/oc/oc /usr/bin/oc
# Build the commatrix binary
RUN go mod vendor && \
    make build && \
    make install INSTALL_DIR=/usr/bin/
