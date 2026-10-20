# Dockerfile for Michelangelo services.

# Distroless: https://github.com/GoogleContainerTools/distroless
# Pinned by digest (rather than the floating `latest` tag) so Dependabot can
# track and propose base-image updates -- see .github/dependabot.yml's
# "docker" entry.
#
# Uses the `-nossl` variant: apiserver/worker/controllermgr are fully static
# Go binaries (`-linkmode external -extldflags -static`, see
# go/cmd/{apiserver,worker,controllermgr}/BUILD.bazel) with no cgo dependency
# on OpenSSL -- confirmed via `readelf -d` against real linux/amd64 builds of
# all three ("There is no dynamic section in this file" for each). Go's
# crypto/tls is pure-Go and never calls into libssl.so.3. The regular `base`
# variant ships libssl3/libcrypto3 even though nothing here links against
# them, which is the only reason CVE-2026-84782 (libssl3, unfixed on Debian
# 12/bookworm -- OpenSSL 3.0 is EOL and the backport is paywalled) ever
# showed up in a scan of these images. `base-nossl` is identical to `base`
# minus libssl/libcrypto; ca-certificates (needed for outbound TLS trust
# verification via crypto/x509) is still included.
FROM gcr.io/distroless/base-nossl-debian12:latest@sha256:e35893f8ca1cabfbd60394af94899c20930c4615355adf97c5da660a51b61bca

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
