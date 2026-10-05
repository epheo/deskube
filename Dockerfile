# Build the manager binary
ARG FEDORA=latest
FROM registry.fedoraproject.org/fedora:${FEDORA} AS builder
ARG TARGETOS
ARG TARGETARCH
RUN dnf -y install golang && dnf clean all

WORKDIR /workspace
COPY go.mod go.mod
COPY go.sum go.sum
# Dependencies in their own layer: source changes do not download them again.
RUN go mod download

COPY cmd/main.go cmd/main.go
COPY api/ api/
COPY internal/ internal/

# GOARCH is left to the platform the build runs for.
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o manager cmd/main.go

# The binary is static and talks only to the API server, whose CA is mounted.
FROM scratch
COPY --from=builder /workspace/manager /manager
USER 65532:65532

ENTRYPOINT ["/manager"]
