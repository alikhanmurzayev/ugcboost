FROM node:20-alpine AS builder
WORKDIR /build
COPY frontend/package.json frontend/package-lock.json ./
COPY frontend/tailwind.preset.ts ./
COPY frontend/web/package.json frontend/web/.npmrc ./web/
RUN npm ci -w web
COPY frontend/web/ ./web/
WORKDIR /build/web
RUN npm run build

FROM nginx:alpine
COPY --from=builder /build/web/dist /usr/share/nginx/html
COPY frontend/web/nginx.conf /etc/nginx/conf.d/default.conf
COPY frontend/web/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
EXPOSE 80
ENTRYPOINT ["/entrypoint.sh"]
