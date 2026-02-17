
# Make kubectl & oc scripts available for copy
FROM registry.redhat.io/openshift4/ose-cli-rhel9:v4.20@sha256:c9d31cdb73cc66f049550773c21097dcce0f4377704113fa4c59744e66ca5fdf AS ose-cli

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
# Build oc-commatrix-mac for macOS arm64
RUN CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -mod=vendor -a -trimpath -ldflags="-s -w"  \
    -o build/oc-commatrix-mac ./cmd/main.go

# Create final image from UBI + built binary and oc
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest@sha256:c7d44146f826037f6873d99da479299b889473492d3c1ab8af86f08af04ec8a0

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
LABEL cpe="cpe:/a:redhat:openshift:4.21::el9"

USER 65532:65532

ENTRYPOINT ["/oc-commatrix"]
