#!/bin/bash
# Se ejecuta una única vez, en el primer arranque con el volumen vacío.
#
# POSTGRES_DB ya creó 'mirror'. Aquí añadimos la base que Evolution API gestiona
# con su propio Prisma. Van separadas a propósito: sus migraciones no deben poder
# tocar nuestras tablas, y un `prisma migrate reset` accidental no nos borra el
# historial de conversaciones.
set -euo pipefail

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
	CREATE DATABASE evolution OWNER $POSTGRES_USER;
EOSQL

echo "[init] base 'evolution' creada"
