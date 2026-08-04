-- Panel espejo de WhatsApp — esquema inicial.
-- Base: 'mirror'. Single-tenant: sin tenant_id en ninguna tabla.
--
-- Modelo mental: los vendedores NO son usuarios. Un vendedor es una `instance`
-- (su línea de WhatsApp). `users` contiene solo administradores del panel.

BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;   -- gen_random_uuid()
CREATE EXTENSION IF NOT EXISTS citext;     -- emails case-insensitive
CREATE EXTENSION IF NOT EXISTS pg_trgm;    -- búsqueda por subcadena en mensajes

-- =============================================================================
-- Tipos
-- =============================================================================

CREATE TYPE user_role         AS ENUM ('admin', 'auditor');
CREATE TYPE instance_status   AS ENUM ('created', 'connecting', 'open', 'close');
CREATE TYPE message_direction AS ENUM ('inbound', 'outbound');

-- El eje de la auditoría. Un saliente puede venir del teléfono del vendedor o
-- del panel, y el webhook de WhatsApp no los distingue (ambos son fromMe:true).
--   contact -> lo escribió el cliente final
--   device  -> lo escribió el vendedor desde su móvil
--   panel   -> lo escribió un admin desde esta aplicación
CREATE TYPE message_origin AS ENUM ('contact', 'device', 'panel');

CREATE TYPE message_type AS ENUM (
    'text', 'image', 'audio', 'video', 'document', 'sticker',
    'location', 'contact', 'reaction', 'system', 'unsupported'
);

CREATE TYPE message_status AS ENUM ('pending', 'sent', 'delivered', 'read', 'failed');
CREATE TYPE media_status   AS ENUM ('pending', 'processing', 'stored', 'failed');
CREATE TYPE event_status   AS ENUM ('pending', 'processed', 'failed', 'skipped');

-- Mantiene updated_at sin depender de que la aplicación se acuerde.
CREATE OR REPLACE FUNCTION touch_updated_at() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$;

-- =============================================================================
-- users — administradores del panel
-- =============================================================================

CREATE TABLE users (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email         citext      NOT NULL UNIQUE,
    password_hash text        NOT NULL,           -- bcrypt
    full_name     text        NOT NULL,
    role          user_role   NOT NULL DEFAULT 'auditor',
    is_active     boolean     NOT NULL DEFAULT true,
    last_login_at timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE users IS
    'Administradores del panel. Los vendedores NO están aquí: usan WhatsApp nativo.';
COMMENT ON COLUMN users.role IS
    'admin = lee y envía mensajes. auditor = solo lectura.';

CREATE TRIGGER users_touch BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

-- =============================================================================
-- instances — líneas de WhatsApp conectadas (una por vendedor)
-- =============================================================================

CREATE TABLE instances (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Nombre de la instancia en Evolution API. Es la clave de correlación entre
    -- los dos sistemas: llega en cada webhook y se usa en cada URL de envío.
    evolution_name text NOT NULL UNIQUE,

    -- Nombre del vendedor dueño del teléfono. Sustituye a lo que en un CRM
    -- colaborativo sería una fila en `users`.
    owner_label   text NOT NULL,

    display_name  text,                            -- etiqueta libre: "Ventas Europa"
    phone_number  text,                            -- E.164, se rellena al conectar
    phone_jid     text,                            -- 573001112233@s.whatsapp.net
    profile_name  text,
    profile_pic_url text,

    status        instance_status NOT NULL DEFAULT 'created',
    is_active     boolean     NOT NULL DEFAULT true,
    last_seen_at  timestamptz,                     -- último evento recibido
    connected_at  timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE instances IS
    'Cada fila es una línea de WhatsApp = un vendedor. El filtro principal del panel.';

CREATE INDEX instances_active_idx ON instances (is_active) WHERE is_active;

CREATE TRIGGER instances_touch BEFORE UPDATE ON instances
    FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

-- =============================================================================
-- contacts — clientes finales
-- =============================================================================
-- Global por JID, no por instancia: si el mismo cliente escribe a dos vendedores
-- queremos poder detectarlo. La separación por línea vive en `conversations`.

CREATE TABLE contacts (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    wa_jid       text NOT NULL UNIQUE,             -- 573001112233@s.whatsapp.net
    phone_number text,                             -- E.164 normalizado
    push_name    text,                             -- nombre que publica el propio contacto
    display_name text,                             -- nombre editado por el admin; tiene prioridad
    profile_pic_url text,
    notes        text,
    is_blocked   boolean     NOT NULL DEFAULT false,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX contacts_phone_idx ON contacts (phone_number);
CREATE INDEX contacts_name_trgm_idx ON contacts
    USING gin (coalesce(display_name, push_name, '') gin_trgm_ops);

CREATE TRIGGER contacts_touch BEFORE UPDATE ON contacts
    FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

-- =============================================================================
-- conversations — thread materializado por (línea, contacto)
-- =============================================================================
-- Existe para que la bandeja ordene y pagine sin escanear `messages`.
-- Sin assigned_user_id: no hay asignación en un panel de auditoría.

CREATE TABLE conversations (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    instance_id uuid NOT NULL REFERENCES instances (id) ON DELETE CASCADE,
    contact_id  uuid NOT NULL REFERENCES contacts  (id) ON DELETE CASCADE,

    remote_jid  text    NOT NULL,                  -- redundante pero evita un JOIN al enviar
    is_group    boolean NOT NULL DEFAULT false,

    -- Denormalizaciones que mantiene el worker en la misma transacción que el
    -- INSERT del mensaje.
    last_message_at      timestamptz,
    last_message_preview text,
    last_message_type    message_type,
    unread_count integer NOT NULL DEFAULT 0,       -- "no visto por el admin"

    is_archived boolean     NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT conversations_line_contact_uniq UNIQUE (instance_id, contact_id)
);

COMMENT ON COLUMN conversations.unread_count IS
    'Pendientes de revisar por el ADMIN, no por el vendedor. El vendedor ya los leyó en su móvil.';

-- La consulta central del producto: bandeja de una línea, más reciente primero.
CREATE INDEX conversations_inbox_idx
    ON conversations (instance_id, last_message_at DESC NULLS LAST)
    WHERE NOT is_archived;

-- Bandeja unificada de todas las líneas.
CREATE INDEX conversations_global_inbox_idx
    ON conversations (last_message_at DESC NULLS LAST)
    WHERE NOT is_archived;

CREATE INDEX conversations_contact_idx ON conversations (contact_id);

CREATE TRIGGER conversations_touch BEFORE UPDATE ON conversations
    FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

-- =============================================================================
-- messages
-- =============================================================================

CREATE TABLE messages (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id uuid NOT NULL REFERENCES conversations (id) ON DELETE CASCADE,

    -- Desnormalizado a propósito: filtrar por línea es LA consulta del panel y no
    -- debe pagar un JOIN contra conversations.
    instance_id     uuid NOT NULL REFERENCES instances (id) ON DELETE CASCADE,
    contact_id      uuid NOT NULL REFERENCES contacts  (id) ON DELETE CASCADE,

    wa_message_id   text NOT NULL,                 -- key.id de Evolution
    direction       message_direction NOT NULL,
    origin          message_origin    NOT NULL,
    type            message_type      NOT NULL,
    status          message_status    NOT NULL DEFAULT 'sent',

    -- Solo si origin='panel'. Un saliente 'device' no tiene autor en el sistema:
    -- lo escribió el vendedor desde su teléfono.
    sent_by_user_id uuid REFERENCES users (id) ON DELETE SET NULL,

    body            text,                          -- texto o caption del media
    quoted_wa_message_id text,

    -- Timestamp de WhatsApp, no de recepción. Un webhook reintentado 40 s después
    -- no debe alterar el orden real de la conversación.
    wa_timestamp    timestamptz NOT NULL,
    edited_at       timestamptz,
    deleted_at      timestamptz,                   -- soft delete: auditamos borrados
    created_at      timestamptz NOT NULL DEFAULT now(),

    -- Idempotencia frente a los reintentos de Evolution.
    CONSTRAINT messages_wa_id_uniq UNIQUE (instance_id, wa_message_id),

    -- Un mensaje del panel siempre tiene autor; ninguno de los otros orígenes
    -- puede tenerlo. Invariante de auditoría, aplicada por la base.
    CONSTRAINT messages_panel_has_author CHECK (
        (origin = 'panel' AND sent_by_user_id IS NOT NULL) OR
        (origin <> 'panel' AND sent_by_user_id IS NULL)
    ),
    CONSTRAINT messages_origin_matches_direction CHECK (
        (direction = 'inbound'  AND origin  = 'contact') OR
        (direction = 'outbound' AND origin <> 'contact')
    )
);

-- Scroll de una conversación (más reciente primero, con keyset pagination).
CREATE INDEX messages_thread_idx
    ON messages (conversation_id, wa_timestamp DESC, id DESC);

-- Timeline completo de una línea: "qué hizo este vendedor esta semana".
CREATE INDEX messages_instance_timeline_idx
    ON messages (instance_id, wa_timestamp DESC);

-- Auditoría directa: todo lo enviado desde el panel.
CREATE INDEX messages_panel_sent_idx
    ON messages (sent_by_user_id, wa_timestamp DESC)
    WHERE origin = 'panel';

-- Búsqueda por subcadena. pg_trgm en vez de tsvector porque los vendedores
-- buscan códigos de reserva y fragmentos de número, no lenguaje natural.
CREATE INDEX messages_body_trgm_idx ON messages
    USING gin (body gin_trgm_ops) WHERE body IS NOT NULL;

-- =============================================================================
-- media_assets — archivos subidos a R2
-- =============================================================================
-- Fila separada de `messages` porque el ciclo de vida es distinto: el mensaje se
-- muestra en el panel de inmediato y el archivo llega después, tras descargarlo
-- de Evolution y subirlo a R2.

CREATE TABLE media_assets (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id  uuid NOT NULL UNIQUE REFERENCES messages (id) ON DELETE CASCADE,

    mime_type   text,
    file_name   text,                              -- nombre original del documento
    size_bytes  bigint,
    sha256      text,                              -- deduplicación y verificación
    duration_seconds integer,                      -- audios .ogg y vídeo
    width       integer,
    height      integer,

    -- Clave en R2: {instance}/{yyyy-mm}/{wa_message_id}.{ext}
    -- Nunca se expone al navegador; el backend firma una URL temporal.
    storage_key text,
    storage_status media_status NOT NULL DEFAULT 'pending',
    attempts    integer     NOT NULL DEFAULT 0,
    last_error  text,
    stored_at   timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

COMMENT ON COLUMN media_assets.storage_key IS
    'Ruta en R2. El frontend pide GET /api/media/{id} y recibe una URL firmada de vida corta.';

-- Cola de trabajo del pipeline de media: los pendientes y los que fallaron.
CREATE INDEX media_assets_pending_idx
    ON media_assets (storage_status, created_at)
    WHERE storage_status IN ('pending', 'failed');

CREATE TRIGGER media_assets_touch BEFORE UPDATE ON media_assets
    FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

-- =============================================================================
-- outbound_messages — envíos originados en el panel
-- =============================================================================
-- Registro de intención, anterior a la confirmación de WhatsApp. Permite
-- reconciliar si el proceso muere entre el POST a Evolution y el webhook, y deja
-- constancia de los envíos fallidos (que nunca generan fila en `messages`).

CREATE TABLE outbound_messages (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id uuid NOT NULL REFERENCES conversations (id) ON DELETE CASCADE,
    instance_id     uuid NOT NULL REFERENCES instances (id) ON DELETE CASCADE,
    user_id         uuid NOT NULL REFERENCES users (id) ON DELETE RESTRICT,

    -- Generado por el cliente. Evita envíos duplicados si el admin hace doble
    -- clic o el navegador reintenta el POST.
    idempotency_key text NOT NULL UNIQUE,

    type            message_type NOT NULL DEFAULT 'text',
    body            text,
    media_storage_key text,                        -- si el admin adjunta un archivo

    -- key.id que devuelve Evolution. Es el puente hacia el webhook: cuando llegue
    -- un fromMe:true con este id, se marcará origin='panel'.
    wa_message_id   text,
    status          message_status NOT NULL DEFAULT 'pending',
    error           text,
    sent_at         timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX outbound_messages_wa_id_idx ON outbound_messages (wa_message_id)
    WHERE wa_message_id IS NOT NULL;
CREATE INDEX outbound_messages_audit_idx ON outbound_messages (user_id, created_at DESC);

-- =============================================================================
-- webhook_events — bitácora cruda e idempotencia
-- =============================================================================
-- Primera parada de todo webhook. El INSERT decide si el evento es nuevo: si el
-- índice único lo rechaza, es un reintento de Evolution y se descarta sin llegar
-- al pool de workers.

CREATE TABLE webhook_events (
    id            bigserial PRIMARY KEY,
    instance_name text  NOT NULL,                  -- crudo; puede no existir aún en `instances`
    event         text  NOT NULL,                  -- messages.upsert, connection.update, ...

    -- {instance}:{event}:{key.id}. Único por naturaleza del protocolo.
    dedupe_key    text  NOT NULL UNIQUE,

    payload       jsonb NOT NULL,
    status        event_status NOT NULL DEFAULT 'pending',
    attempts      integer      NOT NULL DEFAULT 0,
    last_error    text,
    received_at   timestamptz  NOT NULL DEFAULT now(),
    processed_at  timestamptz
);

COMMENT ON TABLE webhook_events IS
    'Bitácora forense + barrera de idempotencia. Se puede purgar pasados 30-90 días.';

CREATE INDEX webhook_events_pending_idx ON webhook_events (received_at)
    WHERE status IN ('pending', 'failed');
CREATE INDEX webhook_events_instance_idx ON webhook_events (instance_name, received_at DESC);

COMMIT;
