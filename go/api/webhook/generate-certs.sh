#!/bin/bash

################################################################################
# Webhook Certificate Generator
#
# CONTEXT:
# The Michelangelo webhook server uses HTTPS to receive conversion requests
# from the Kubernetes API server. HTTPS requires TLS certificates.
#
# WHY WE NEED OUR OWN CA:
# - Public CAs (like Let's Encrypt) are for public internet, not internal K8s clusters
# - We create our own Certificate Authority (CA) to sign our webhook certificates
# - K8s API server trusts our CA via the CABundle in the CRD's webhook config
#
# WHAT THIS SCRIPT GENERATES:
# 1. ca.crt & ca.key     - Our own Certificate Authority (the "trust anchor")
# 2. tls.crt & tls.key   - Webhook server certificate signed by our CA
#
# HOW IT WORKS:
# - Controller-runtime (webhook server) uses tls.crt + tls.key for HTTPS
# - K8s API server receives ca.crt via WebhookConversion config to trust our certs
#
# WHEN TO RUN:
# - First time setting up local development
# - When certificates expire (default: 1 year)
# - When you delete the certs directory
#
# USAGE:
#   ./go/api/webhook/generate-certs.sh
#
################################################################################

set -euo pipefail

# Directory where certificates will be generated
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CERT_DIR="$SCRIPT_DIR/certs"

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
echo "You can now run the apiserver:"
echo "  bazel run //go/cmd/apiserver:apiserver"
