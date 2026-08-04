-- Kernel — esquema compartido.
--
-- Aquí vive solo lo que sirve a más de un módulo: las extensiones, el trigger de
-- updated_at y el directorio de personal. Las tablas de cada módulo van en sus
-- propias migraciones.
--
-- Este archivo se aplica ANTES que el de cualquier módulo: los módulos tienen
-- claves foráneas contra `users`.

-- Las extensiones son idempotentes a propósito: cualquier módulo puede darlas por
-- supuestas sin declararlas.
CREATE EXTENSION IF NOT EXISTS pgcrypto;   -- gen_random_uuid()
CREATE EXTENSION IF NOT EXISTS citext;     -- emails case-insensitive
CREATE EXTENSION IF NOT EXISTS pg_trgm;    -- búsqueda por subcadena

-- Mantiene updated_at sin depender de que la aplicación se acuerde. Compartida:
-- los módulos solo tienen que colgar el trigger.
CREATE OR REPLACE FUNCTION touch_updated_at() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$;

-- =============================================================================
-- users — directorio del personal de la agencia
-- =============================================================================
-- Todo el que trabaja en la agencia y entra al panel, vendedores incluidos.
--
-- El rol NO controla el acceso a módulos: todos los usuarios entran a todos los
-- módulos. Solo distingue quién puede administrar usuarios.

CREATE TYPE user_role AS ENUM ('admin', 'member');

CREATE TABLE users (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email         citext      NOT NULL UNIQUE,
    password_hash text        NOT NULL,           -- bcrypt
    full_name     text        NOT NULL,
    role          user_role   NOT NULL DEFAULT 'member',
    is_active     boolean     NOT NULL DEFAULT true,
    last_login_at timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

COMMENT ON COLUMN users.role IS
    'admin = además gestiona usuarios. member = todo lo demás. No filtra módulos.';
COMMENT ON COLUMN users.is_active IS
    'Baja lógica. Un usuario inactivo no puede autenticarse pero conserva la autoría de sus mensajes.';

CREATE TRIGGER users_touch BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
