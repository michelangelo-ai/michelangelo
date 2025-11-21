#!/bin/bash

################################################################################
# This script generates HTTPS certificate files for testing.
#
# Creates our own Certificate Authority for test purposes and generates:
# 1. ca.crt & ca.key     - Certificate Authority (for K8s API server trust)
# 2. tls.crt & tls.key   - Webhook server certificate
#
# USAGE:
#   ./tools/test/generate-certs.sh [cert_directory]
#
# Arguments:
#   cert_directory - Optional. Directory where certs will be created.
#                    Defaults to ./tools/test/certs if not specified.
#
# Examples:
#   ./tools/test/generate-certs.sh                    # Creates in ./tools/test/certs
#   ./tools/test/generate-certs.sh /tmp/my-certs     # Creates in /tmp/my-certs
################################################################################

set -euo pipefail

# Parse arguments
CERT_DIR="${1:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/certs}"

# Certificate parameters
SERVICE_NAME="michelangelo-webhook-service.default.svc"
CA_VALIDITY_DAYS=3650  # 10 years
CERT_VALIDITY_DAYS=365 # 1 year

echo "==> Generating webhook certificates..."
echo "    Certificate directory: $CERT_DIR"

# Create cert directory
mkdir -p "$CERT_DIR"

# Step 1: Generate CA private key
echo "==> [1/5] Generating CA private key..."
openssl genrsa -out "$CERT_DIR/ca.key" 2048 2>/dev/null

# Step 2: Generate self-signed CA certificate
echo "==> [2/5] Generating CA certificate..."
openssl req -x509 -new -nodes \
  -key "$CERT_DIR/ca.key" \
  -subj "/CN=Michelangelo Webhook CA/O=Michelangelo" \
  -days "$CA_VALIDITY_DAYS" \
  -out "$CERT_DIR/ca.crt" 2>/dev/null

# Step 3: Generate server private key
echo "==> [3/5] Generating server private key..."
openssl genrsa -out "$CERT_DIR/tls.key" 2048 2>/dev/null

# Step 4: Generate certificate signing request (CSR)
echo "==> [4/5] Generating certificate signing request..."
openssl req -new \
  -key "$CERT_DIR/tls.key" \
  -subj "/CN=$SERVICE_NAME/O=Michelangelo" \
  -out "$CERT_DIR/server.csr" 2>/dev/null

# Step 5: Sign server certificate with our CA
echo "==> [5/5] Signing server certificate with CA..."
# Create a config file for SAN (Subject Alternative Names)
cat > "$CERT_DIR/san.conf" <<EOF
[req]
distinguished_name = req_distinguished_name
req_extensions = v3_req

[req_distinguished_name]

[v3_req]
subjectAltName = @alt_names

[alt_names]
DNS.1 = $SERVICE_NAME
DNS.2 = localhost
DNS.3 = host.docker.internal
IP.1 = 127.0.0.1
EOF

openssl x509 -req \
  -in "$CERT_DIR/server.csr" \
  -CA "$CERT_DIR/ca.crt" \
  -CAkey "$CERT_DIR/ca.key" \
  -CAcreateserial \
  -out "$CERT_DIR/tls.crt" \
  -days "$CERT_VALIDITY_DAYS" \
  -extensions v3_req \
  -extfile "$CERT_DIR/san.conf" 2>/dev/null

# Cleanup CSR and SAN config (no longer needed)
rm -f "$CERT_DIR/server.csr" "$CERT_DIR/san.conf"

echo ""
echo "==> ✓ Certificates generated successfully!"
echo ""
echo "Generated files:"
echo "  - $CERT_DIR/ca.crt      (CA certificate - sent to K8s API server)"
echo "  - $CERT_DIR/ca.key      (CA private key)"
echo "  - $CERT_DIR/tls.crt     (Server certificate - used by webhook)"
echo "  - $CERT_DIR/tls.key     (Server private key - used by webhook)"
echo ""
echo "Valid for: $CERT_VALIDITY_DAYS days (until $(date -v+${CERT_VALIDITY_DAYS}d '+%Y-%m-%d' 2>/dev/null || date -d "+${CERT_VALIDITY_DAYS} days" '+%Y-%m-%d' 2>/dev/null || echo 'N/A'))"
echo ""
echo "Next steps:"
echo "  1. Update config/base.yaml webhook.certDir to: $CERT_DIR"
echo "  2. Run: bazel run //go/api/webhook:webhook"
echo "  3. Run: bazel run //go/cmd/apiserver:apiserver"
