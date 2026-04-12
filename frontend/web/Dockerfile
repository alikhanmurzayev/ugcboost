FROM node:20-alpine AS builder
COPY frontend/tailwind.preset.ts /build/
WORKDIR /build/web
COPY frontend/web/package*.json frontend/web/.npmrc ./
RUN npm ci
COPY frontend/web/ .
RUN npm run build

FROM nginx:alpine
COPY --from=builder /build/web/dist /usr/share/nginx/html
COPY frontend/web/nginx.conf /etc/nginx/conf.d/default.conf
COPY frontend/web/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
EXPOSE 80
ENTRYPOINT ["/entrypoint.sh"]
