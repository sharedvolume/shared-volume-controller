# Build the manager binary
FROM golang:1.25 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/main.go cmd/main.go
COPY api/ api/
COPY internal/ internal/

# Build
# the GOARCH has not a default value to allow the binary be built according to the host where the command
# was called. For example, if we call make docker-build in a local env which has the Apple Silicon M1 SO
# the docker BUILDPLATFORM arg will be linux/arm64 when for Apple x86 it will be linux/amd64. Therefore,
# by leaving it empty we can ensure that the container and binary shipped on it will have the same platform.
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o manager cmd/main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot

# Add labels for container metadata
LABEL maintainer="SharedVolume Team info@sharedvolume.io"
LABEL org.opencontainers.image.title="Shared Volume Controller"
LABEL org.opencontainers.image.description="A Kubernetes operator for managing shared volumes with NFS server integration"
LABEL org.opencontainers.image.url="https://github.com/sharedvolume/shared-volume-controller"
LABEL org.opencontainers.image.source="https://github.com/sharedvolume/shared-volume-controller"
LABEL org.opencontainers.image.vendor="SharedVolume"
LABEL org.opencontainers.image.licenses="MIT"

WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
