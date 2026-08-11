# ---------- build stage ----------
FROM golang:1.23-bookworm AS build

WORKDIR /src/backend
# Cache module downloads first
COPY backend/go.mod backend/go.sum ./
RUN go mod download

# Build the static binary (CGO disabled -> portable)
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/feedsocial .

# ---------- runtime stage ----------
FROM debian:bookworm-slim

# ca-certificates and tzdata are required; wget is needed for health checks
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates tzdata wget \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Bake the binary AND the frontend directly into the image.
COPY --from=build /out/feedsocial /app/feedsocial
COPY web/ /app/web/

ENV WEB_DIR=/app/web \
    PORT=8080 \
    FEED_WORKER=on \
    FEED_REFRESH_MINUTES=15

# Expose the internal port only. Coolify's reverse proxy handles public routing.
EXPOSE 8080

CMD ["/app/feedsocial"]
