
# Make kubectl & oc scripts available for copy
FROM registry.redhat.io/openshift4/ose-cli-rhel9:v4.20@sha256:ec38425caa96c78c860a97b28332aa2b7f0d86dce3cc4cfc7a2060c265c55c27 AS ose-cli

# Build the manager binary
FROM brew.registry.redhat.io/rh-osbs/openshift-golang-builder:rhel_9_golang_1.24@sha256:a3a516cd657576fc8462f88c695b2fa87ada72b00416a24c9253f5b3dc6125a4 AS builder

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
# Build oc-commatrix-mac for macOS arm64
RUN CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -mod=vendor -a -trimpath -ldflags="-s -w"  \
    -o build/oc-commatrix-mac ./cmd/main.go

# Create final image from UBI + built binary and oc
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest@sha256:bb08f2300cb8d12a7eb91dddf28ea63692b3ec99e7f0fa71a1b300f2756ea829

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
