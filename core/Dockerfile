# Build stage
FROM golang:1.24-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o h1v3d ./cmd/h1v3d
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o h1v3ctl ./cmd/h1v3ctl

# Runtime stage
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata bash
COPY --from=builder /build/h1v3d /usr/local/bin/
COPY --from=builder /build/h1v3ctl /usr/local/bin/
RUN mkdir -p /data /data/skills
COPY skills/ /data/skills/
RUN find /data/skills -name '*.sh' -exec chmod +x {} +
VOLUME /data
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s \
  CMD wget -qO- http://localhost:8080/api/health || exit 1
CMD ["h1v3d"]
