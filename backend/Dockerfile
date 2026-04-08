FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /api ./cmd/api

FROM alpine:3.20
RUN apk --no-cache add ca-certificates tzdata && \
    adduser -D -u 1000 appuser
USER appuser
WORKDIR /app
COPY --from=builder /api .
EXPOSE 8080
ENTRYPOINT ["./api"]
