#!/bin/bash
# Se ejecuta una única vez, en el primer arranque con el volumen vacío.
#
# POSTGRES_DB ya creó 'mirror'. Evolution Go separa el registro de instancias
# (auth) de las sesiones y claves de WhatsApp (users).
set -euo pipefail

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
	CREATE DATABASE evogo_auth OWNER $POSTGRES_USER;
	CREATE DATABASE evogo_users OWNER $POSTGRES_USER;
EOSQL

echo "[init] bases 'evogo_auth' y 'evogo_users' creadas"
