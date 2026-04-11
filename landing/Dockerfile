FROM node:20-alpine AS builder
WORKDIR /app
COPY landing/package*.json ./
RUN npm ci
COPY landing/ .
RUN npm run build

FROM nginx:alpine
COPY --from=builder /app/dist /usr/share/nginx/html
COPY landing/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
EXPOSE 80
ENTRYPOINT ["/entrypoint.sh"]
