# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS build
WORKDIR /src

# Capa de dependencias separada: cambiar código no reinvalida el módulo cacheado.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
# CGO_ENABLED=0 para un binario estático que corre en una imagen sin libc.
# -trimpath quita las rutas del build host de los mensajes de error.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

FROM alpine:3.20
# ca-certificates: R2 es HTTPS. tzdata: los timestamps se muestran en la zona de
# la agencia, no en UTC.
RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -u 10001 app
WORKDIR /app
COPY --from=build /out/api /app/api
COPY --from=build /src/migrations /app/migrations

USER app
EXPOSE 8080
ENTRYPOINT ["/app/api"]
