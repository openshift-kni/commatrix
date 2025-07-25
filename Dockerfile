FROM golang:1.22 AS builder

WORKDIR /go/src/github.com/openshift-kni/commatrix
COPY . .

# Build the binary
RUN make build && \
    mkdir -p /tmp/build && \
    cp oc-commatrix /tmp/build/

FROM alpine:latest
COPY --from=builder /tmp/build/oc-commatrix /usr/local/bin/
