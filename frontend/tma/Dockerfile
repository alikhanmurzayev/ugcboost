FROM node:20-alpine AS builder
WORKDIR /build
COPY frontend/package.json frontend/package-lock.json ./
COPY frontend/tailwind.preset.ts ./
COPY frontend/tma/package.json frontend/tma/.npmrc ./tma/
RUN npm ci -w tma
COPY frontend/tma/ ./tma/
WORKDIR /build/tma
RUN npm run build

FROM nginx:alpine
COPY --from=builder /build/tma/dist /usr/share/nginx/html
COPY frontend/tma/nginx.conf /etc/nginx/conf.d/default.conf
COPY frontend/tma/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
EXPOSE 80
ENTRYPOINT ["/entrypoint.sh"]
