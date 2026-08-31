# Simple UI Dockerfile - expects pre-built assets
# Pinned by digest (rather than the floating `alpine` tag) so Dependabot can
# track and propose base-image updates -- see .github/dependabot.yml's
# "docker" entry.
FROM nginx:alpine@sha256:db35bfc6b2951e7f8a72db5db120288c127ffaeeb4a6d4b95a26fead017d5913

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