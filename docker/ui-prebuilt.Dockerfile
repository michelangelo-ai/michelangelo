# Simple UI Dockerfile - expects pre-built assets
# Pinned by digest (rather than the floating `alpine` tag) so Dependabot can
# track and propose base-image updates -- see .github/dependabot.yml's
# "docker" entry.
FROM nginx:alpine@sha256:62ff2089abf5a9ed33bd232895bef5e22f7bb4b200675cec49a5ebc48e3d4ac8

# Upgrade every installed Alpine package rather than a named list. The base is
# pinned by digest (see above), so it only moves when Dependabot bumps it, while
# Alpine publishes fixes for nginx's transitive libraries (brotli-libs, curl,
# libcurl, libexpat, libx11, pcre2, tiff, ...) continuously. A named list needs
# a new Dockerfile edit for every CVE scan finding (#2162, #2233); upgrading
# everything clears them all at build time. Exact -rN versions are deliberately
# not pinned, since they stop resolving once Alpine moves on.
RUN apk upgrade --no-cache

# Copy pre-built UI assets
COPY javascript/app/dist /usr/share/nginx/html

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