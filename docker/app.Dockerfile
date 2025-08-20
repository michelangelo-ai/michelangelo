# Use Node.js 22.11.0 to match the engines requirement
FROM node:22.11.0-alpine AS builder

# Install dependencies for the gen-grpc-client.sh script
RUN apk add --no-cache bash git perl curl

# Install buf (protobuf tool) - use same version as dev environment
RUN curl -sSL "https://github.com/bufbuild/buf/releases/download/v1.50.0/buf-$(uname -s)-$(uname -m)" -o "/usr/local/bin/buf" && \
    chmod +x "/usr/local/bin/buf"

WORKDIR /workspace

# Copy necessary files for the build
COPY javascript/ ./javascript/
COPY tools/ ./tools/
COPY proto/ ./proto/

# Change to javascript directory and run setup
WORKDIR /workspace/javascript
ENV WORKSPACE_ROOT=/workspace
RUN yarn setup
RUN yarn workspace @michelangelo/app build

# Production image
FROM nginx:alpine

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