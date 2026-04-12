FROM node:20-alpine AS builder
COPY frontend/tailwind.preset.ts /build/
WORKDIR /build/tma
COPY frontend/tma/package*.json frontend/tma/.npmrc ./
RUN npm ci
COPY frontend/tma/ .
RUN npm run build

FROM nginx:alpine
COPY --from=builder /build/tma/dist /usr/share/nginx/html
COPY frontend/tma/nginx.conf /etc/nginx/conf.d/default.conf
COPY frontend/tma/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
EXPOSE 80
ENTRYPOINT ["/entrypoint.sh"]
