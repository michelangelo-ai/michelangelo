# Dockerfile for Michelangelo services.

# Distroless: https://github.com/GoogleContainerTools/distroless
# Pinned by digest (rather than the floating `latest` tag) so Dependabot can
# track and propose base-image updates -- see .github/dependabot.yml's
# "docker" entry.
FROM gcr.io/distroless/base-debian12:latest@sha256:fabbf1c0c357a3d42550111351daed089b20a2c954df13ee2fcff60602515e84

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
