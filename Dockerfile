# Imagen de la app Go de RadioClub25 (multi-stage).
# 1) build: compila un binario estático (sin CGO gracias a modernc.org/sqlite).
# 2) final: alpine mínima con certificados y zona horaria.

FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/radioweb ./cmd/radioweb

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata wget \
    && adduser -D -u 10001 app \
    && mkdir -p /data && chown app:app /data
COPY --from=build /out/radioweb /usr/local/bin/radioweb
USER app
ENV DATA_DIR=/data HTTP_ADDR=:8120
EXPOSE 8120
VOLUME ["/data"]
HEALTHCHECK --interval=30s --timeout=3s --retries=3 CMD wget -qO- http://127.0.0.1:8120/healthz || exit 1
ENTRYPOINT ["radioweb"]
CMD ["serve"]
