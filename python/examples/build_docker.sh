#!/usr/bin/env bash
set -e

#Specify the dockerfile template to use 

dockerfile_template_path="$1"
image_tag="$2"

# check the required arguments are provided
if [[ -z "$dockerfile_template_path" || -z "$image_tag" ]]; then
  echo "Usage: $0 <dockerfile_template_path> <image_tag>"
  exit 1
fi

CERT_FILE="$CUSTOM_CA_PATH"

if [[ -f "$CERT_FILE" ]]; then
  echo "🔐 Using custom CA: $CERT_FILE"
  export CA_MOUNT_BLOCK="RUN --mount=type=secret,id=custom-ca,target=/tmp/custom-ca.pem \
  cp /tmp/custom-ca.pem /usr/local/share/ca-certificates/custom-ca.crt && \
  update-ca-certificates"

  export CA_ENV_BLOCK="ENV SSL_CERT_FILE=/usr/local/share/ca-certificates/custom-ca.crt
ENV SSL_CERT_DIR=/usr/local/share/ca-certificates
ENV REQUESTS_CA_BUNDLE=/usr/local/share/ca-certificates/custom-ca.crt
ENV CERT_PATH=/usr/local/share/ca-certificates/custom-ca.crt 
ENV CERT_DIR=/usr/local/share/ca-certificates"
else
  echo "⚠️  No CA cert found, building without CA"
  export CA_MOUNT_BLOCK="RUN echo 'Skipping CA injection'"
  export CA_ENV_BLOCK="RUN echo 'Skipping CA environment'"
fi

# cmd to replace the dockerfile template with the CA mount and env blocks
dockerfile_template_with_ca_blocks=$(envsubst '${CA_MOUNT_BLOCK} ${CA_ENV_BLOCK}' < $dockerfile_template_path)

docker_build_cmd="docker buildx build -t $image_tag --progress=auto -f - . "

if [[ -f "$CERT_FILE" ]]; then
  docker_build_cmd="$docker_build_cmd --secret id=custom-ca,src=$CERT_FILE"
fi

echo "$dockerfile_template_with_ca_blocks" | $docker_build_cmd 