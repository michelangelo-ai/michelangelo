# Use Node.js 24.18.0 to match the engines requirement
FROM node:24.18.0-alpine AS builder

# Install dependencies for the gen-grpc-client.sh script
RUN apk add --no-cache bash git perl curl

# Install buf (protobuf tool) for gen-grpc-client.sh script
RUN curl -sSL "https://github.com/bufbuild/buf/releases/download/v1.50.0/buf-$(uname -s)-$(uname -m)" -o "/usr/local/bin/buf" && \
    chmod +x "/usr/local/bin/buf"

WORKDIR /workspace

# javascript includes application code
COPY javascript/ ./javascript/

# tools includes the gen-grpc-client.sh script
COPY tools/ ./tools/

# proto includes the protobuf files that are used to generate the grpc client
COPY proto/ ./proto/

# Build the app
WORKDIR /workspace/javascript
ENV WORKSPACE_ROOT=/workspace
RUN yarn setup
RUN yarn workspace @michelangelo/app build

# Production image
# Pinned by digest (rather than the floating `alpine` tag) so Dependabot can
# track and propose base-image updates -- see .github/dependabot.yml's
# "docker" entry.
FROM nginx:alpine@sha256:a9ae6f6d078d477e21323310498e5196cb2b7c0aedd9e07b7306612077227d7c

# Copy built app to nginx
COPY --from=builder /workspace/javascript/app/dist /usr/share/nginx/html

# Create nginx config for React Router
RUN echo 'server { \
    listen 80; \
    location / { \
        root /usr/share/nginx/html; \
        index index.html index.htm; \
        try_files $uri $uri/ /index.html; \
    } \
}' > /etc/nginx/conf.d/default.conf

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
