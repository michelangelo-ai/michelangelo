# Dockerfile for Michelangelo services.

# Distroless: https://github.com/GoogleContainerTools/distroless
FROM debian:12

# Path to the service binary built by the BAZEL_TARGET.
# The path must be relative to the repository root.
# Ex: go/cmd/controllermgr/controllermgr_/controllermgr
ARG BINARY_PATH

# Path to the service config directory.
# The path must be relative to the repository root.
# Ex: bazel-bin/go/cmd/controllermgr/config
ARG CONFIG_PATH

COPY $BINARY_PATH /app
COPY $CONFIG_PATH /config

ENTRYPOINT ["/app"]
