# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:alpine AS builder

ARG TARGETOS
ARG TARGETARCH
ENV GOTOOLCHAIN=auto

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o /out/ecs-cdt-panel .

FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S app \
    && adduser -S -G app app \
    && mkdir -p /app/data \
    && chown -R app:app /app

WORKDIR /app

COPY --from=builder /out/ecs-cdt-panel /app/ecs-cdt-panel

ENV PORT=8080 \
    DATA_DIR=/app/data \
    TZ=Asia/Shanghai \
    CHECK_INTERVAL=60 \
    PANEL_USERNAME=admin \
    GOMEMLIMIT=48MiB

EXPOSE 8080
VOLUME ["/app/data"]

USER app

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD wget -qO- http://127.0.0.1:${PORT}/api/health >/dev/null || exit 1

ENTRYPOINT ["/app/ecs-cdt-panel"]
