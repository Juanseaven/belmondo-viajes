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
# Solo el binario: las migraciones van embebidas con go:embed dentro de cada
# módulo, así que el esquema y el código que lo asume no se pueden desincronizar
# al desplegar.
COPY --from=build /out/api /app/api

# El almacenamiento local escribe acá cuando no hay R2. Se crea con el dueño
# correcto en el build: el proceso corre sin privilegios y no podría crearlo.
RUN mkdir -p /data/blobs && chown -R app /data/blobs

USER app
EXPOSE 8080
ENTRYPOINT ["/app/api"]
