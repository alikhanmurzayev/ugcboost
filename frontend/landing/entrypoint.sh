#!/bin/sh
# Inject runtime config into landing before starting nginx.
# API_URL env var is set per-environment (staging, production, local Docker).
cat > /usr/share/nginx/html/config.js <<EOF
window.__RUNTIME_CONFIG__ = { apiUrl: "${API_URL}" };
EOF
exec nginx -g 'daemon off;'
