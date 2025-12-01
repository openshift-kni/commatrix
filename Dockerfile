
# Make kubectl & oc scripts available for copy
FROM registry.redhat.io/openshift4/ose-cli-rhel9:v4.20@sha256:b8513a2ae627fa6d748f7e312c2439b7581de41ca0c41e5f01e35f89b9de63e9 AS ose-cli

# Build the manager binary
FROM brew.registry.redhat.io/rh-osbs/openshift-golang-builder:rhel_9_golang_1.24@sha256:8412323224878c4e27cd6f7dc6ce99c50f878f37df323285bff29a53f8ec37cd AS builder

WORKDIR /opt/app-root

COPY go.mod go.mod
COPY go.sum go.sum
COPY cmd cmd
COPY pkg pkg
COPY vendor/ vendor/

# Build oc-commatrix
ENV GOEXPERIMENT=strictfipsruntime
ENV CGO_ENABLED=1
RUN mkdir -p build && \
    go build -mod=vendor -tags strictfipsruntime -a -trimpath -ldflags="-s -w" \
      -o build/oc-commatrix ./cmd/main.go
# Create final image from UBI + built binary and oc
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest@sha256:161a4e29ea482bab6048c2b36031b4f302ae81e4ff18b83e61785f40dc576f5d

WORKDIR /

COPY --from=builder /opt/app-root/build .
COPY --from=ose-cli /usr/bin/oc /usr/bin/oc

COPY LICENSE /licenses/
COPY README.md ./README

# Required labels for Conforma/Red Hat
LABEL name="openshift-kni/commatrix"
LABEL summary="Communication Matrix CLI"
LABEL description="CLI to generate OpenShift communication matrices and optional host open ports diff."
LABEL io.k8s.display-name="Communication Matrix CLI"
LABEL io.k8s.description="Communication Matrix CLI"
LABEL io.openshift.tags="commatrix,oc,cli"
LABEL maintainer="support@redhat.com"
LABEL com.redhat.component="commatrix"
LABEL version="4.21"
LABEL release="1"
LABEL vendor="Red Hat, Inc."
LABEL url="https://github.com/openshift-kni/commatrix"
LABEL distribution-scope="public"

USER 65532:65532

ENTRYPOINT ["/oc-commatrix"]
