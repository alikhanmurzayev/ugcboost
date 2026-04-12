FROM node:20-alpine AS builder
WORKDIR /build
COPY frontend/package.json frontend/package-lock.json ./
COPY frontend/landing/package.json ./landing/
RUN npm ci -w landing && npm install @rollup/rollup-linux-x64-musl --no-save
COPY frontend/landing/ ./landing/
WORKDIR /build/landing
RUN npm run build

FROM nginx:alpine
COPY --from=builder /build/landing/dist /usr/share/nginx/html
COPY frontend/landing/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
EXPOSE 80
ENTRYPOINT ["/entrypoint.sh"]
