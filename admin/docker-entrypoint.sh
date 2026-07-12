#!/bin/sh
# Render the runtime configuration from environment variables, then start nginx.
# One generic image → any environment: change the env, restart the container.
set -eu

# Defaults for every documented runtime variable. Override any of these by
# passing the matching environment variable to `docker run` / compose.
: "${API_BASE_URL:=http://localhost:8080}"
: "${TENANT_MODE:=header}"
: "${TENANT_SLUG:=}"
: "${APP_NAME:=Entitlements}"
: "${ENABLE_DEMO:=false}"

export API_BASE_URL TENANT_MODE TENANT_SLUG APP_NAME ENABLE_DEMO

template=/usr/share/nginx/html/app-config.js.template
target=/usr/share/nginx/html/app-config.js

# Use the envsubst that ships with nginx:alpine. Restrict substitution to our
# known variables so any other `$` in the file is left untouched.
envsubst '${API_BASE_URL} ${TENANT_MODE} ${TENANT_SLUG} ${APP_NAME} ${ENABLE_DEMO}' \
  <"$template" >"$target"

echo "Rendered app-config.js:"
cat "$target"

exec nginx -g 'daemon off;'
