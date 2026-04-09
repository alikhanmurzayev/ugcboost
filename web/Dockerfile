FROM node:20-alpine AS builder
COPY tailwind.preset.ts /build/
WORKDIR /build/web
COPY web/package*.json web/.npmrc ./
RUN npm ci
COPY web/ .
RUN npm run build

FROM nginx:alpine
COPY --from=builder /build/web/dist /usr/share/nginx/html
COPY web/nginx.conf /etc/nginx/conf.d/default.conf
COPY web/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
EXPOSE 80
ENTRYPOINT ["/entrypoint.sh"]
