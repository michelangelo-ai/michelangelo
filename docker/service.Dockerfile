# Dockerfile for Michelangelo services.
#
# Build Examples:
#   Build an image for the controllermgr service:
#   docker build -t michelangelo-controllermgr -f docker/service.Dockerfile --build-arg BAZEL_TARGET=//go/cmd/controllermgr --build-arg BINARY_PATH=bazel-bin/go/cmd/controllermgr/controllermgr_/controllermgr --build-arg CONFIG_PATH=go/cmd/controllermgr/config .
#
#   Build an image for the worker service:
#   docker build -t michelangelo-worker -f docker/service.Dockerfile --build-arg BAZEL_TARGET=//go/cmd/worker --build-arg BINARY_PATH=bazel-bin/go/cmd/worker/worker_/worker --build-arg CONFIG_PATH=go/cmd/worker/config .
FROM debian:12-slim AS build

RUN apt-get update && apt-get install -y --no-install-recommends \
    direnv \
    python3 \
    ca-certificates \
    build-essential \
    curl \
    unzip \
    sudo \
    && rm -rf /var/lib/apt/lists/*

# Bazel target that builds a service into a binary executable.
# Ex: //go/cmd/controllermgr:controllermgr
ARG BAZEL_TARGET

WORKDIR /repo
COPY . .

RUN ./tools/bazel build $BAZEL_TARGET

# Distroless: https://github.com/GoogleContainerTools/distroless
FROM gcr.io/distroless/static-debian12

# Path to the service binary built by the BAZEL_TARGET.
# The path must be relative to the repository root.
# Ex: go/cmd/controllermgr/controllermgr_/controllermgr
ARG BINARY_PATH

# Path to the service config directory.
# The path must be relative to the repository root.
# Ex: bazel-bin/go/cmd/controllermgr/config
ARG CONFIG_PATH

COPY --from=build /repo/$BINARY_PATH /app
COPY --from=build /repo/$CONFIG_PATH /config

ENTRYPOINT ["/app"]
