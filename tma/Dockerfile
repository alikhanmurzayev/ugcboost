FROM node:20-alpine AS builder
COPY tailwind.preset.ts /build/
WORKDIR /build/tma
COPY tma/package*.json tma/.npmrc ./
RUN npm ci
COPY tma/ .
RUN npm run build

FROM nginx:alpine
COPY --from=builder /build/tma/dist /usr/share/nginx/html
COPY tma/nginx.conf /etc/nginx/conf.d/default.conf
COPY tma/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
EXPOSE 80
ENTRYPOINT ["/entrypoint.sh"]
