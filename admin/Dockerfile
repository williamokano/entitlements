# ---------------------------------------------------------------------------
# Build stage — compile the Vite SPA into static assets.
# ---------------------------------------------------------------------------
FROM node:22-alpine AS build
WORKDIR /app

# Install dependencies against the lockfile first for better layer caching.
COPY package.json package-lock.json ./
RUN npm ci

COPY . .

# The ONLY build-time switch: whether the vendored Inspinia demo is bundled
# (it changes what gets tree-shaken out). Every *deploy-time* setting — API URL,
# tenant mode, branding — is injected at container start, never baked in here.
ARG VITE_ENABLE_DEMO=false
ENV VITE_ENABLE_DEMO=${VITE_ENABLE_DEMO}
RUN npm run build

# ---------------------------------------------------------------------------
# Runtime stage — serve the static bundle with nginx, config injected at start.
# ---------------------------------------------------------------------------
FROM nginx:alpine AS runtime

# nginx:alpine already ships `envsubst` (via its gettext libs); do NOT
# `apk add gettext` — it is unnecessary and the alpine CDN may be unreachable.
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html

# The runtime-config template is rendered into app-config.js at container start.
COPY app-config.js.template /usr/share/nginx/html/app-config.js.template
COPY docker-entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

EXPOSE 80
ENTRYPOINT ["/entrypoint.sh"]
