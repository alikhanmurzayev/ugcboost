FROM node:20-alpine AS builder
WORKDIR /app
COPY frontend/landing/package*.json ./
RUN npm ci
COPY frontend/landing/ .
RUN npm run build

FROM nginx:alpine
COPY --from=builder /app/dist /usr/share/nginx/html
COPY frontend/landing/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
EXPOSE 80
ENTRYPOINT ["/entrypoint.sh"]
