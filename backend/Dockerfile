FROM golang:1.25-alpine AS builder

ARG GIT_COMMIT=dev

RUN wget -O /usr/local/bin/goose https://github.com/pressly/goose/releases/download/v3.27.0/goose_linux_x86_64 \
    && chmod +x /usr/local/bin/goose

WORKDIR /app
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build \
        -ldflags="-s -w -X 'github.com/alikhanmurzayev/ugcboost/backend/internal/handler.version=${GIT_COMMIT}'" \
        -o /api ./cmd/api

FROM alpine:3.20
RUN apk --no-cache add ca-certificates tzdata && \
    adduser -D -u 1000 appuser
COPY --from=builder /usr/local/bin/goose /usr/local/bin/goose
USER appuser
WORKDIR /app
COPY --from=builder /api .
COPY migrations/ /migrations/
EXPOSE 8080
ENTRYPOINT ["./api"]
