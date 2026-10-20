-- Módulo requisitos: qué pide cada país a cada nacionalidad.
--
-- Las reglas salen solas de las fuentes —Wikipedia, Passport Index y unas pocas
-- constantes— y nadie las carga ni las aprueba. Lo que las fuentes no cubren o
-- no comparten queda en «verificar» dentro de `fields`, nunca en «OK».
--
-- Tabla pública: entry_rules. La lee el módulo operaciones para evaluar los
-- viajes; nunca la escribe. Tocar sus columnas es cambiar un contrato.

CREATE TABLE entry_rules (
    nationality   text NOT NULL CHECK (nationality ~ '^[A-Z]{2}$'),
    country       text NOT NULL CHECK (country ~ '^[A-Z]{2}$'),
    status        text NOT NULL DEFAULT 'buscando'
                      CHECK (status IN ('buscando', 'vigente', 'sin_fuentes')),
    -- Los campos combinados (domain.Fields): cada uno con valor o «verificar»,
    -- y sus fuentes con la cita.
    fields        jsonb NOT NULL DEFAULT '{}',
    version       int NOT NULL DEFAULT 0,

    -- Lo que hace falta para refrescar sin volver a pagar una extracción: los
    -- campos de Wikipedia antes de combinar y el hash del texto del que salieron.
    wiki_fields   jsonb,
    wiki_hash     text,
    wiki_revision text,
    wiki_url      text,
    -- La fila de Wikipedia cambió hace menos de 72 horas: la regla sigue con el
    -- texto anterior y se vuelve a mirar cada día.
    pending_since timestamptz,
    checked_at    timestamptz,

    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (nationality, country)
);

CREATE INDEX entry_rules_revision_idx ON entry_rules (checked_at NULLS FIRST);

-- Cada cambio de una regla queda guardado, para poder ver por qué un viaje
-- cambió de estado.
CREATE TABLE entry_rule_versions (
    nationality text NOT NULL,
    country     text NOT NULL,
    version     int  NOT NULL,
    status      text NOT NULL,
    fields      jsonb NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (nationality, country, version),
    FOREIGN KEY (nationality, country) REFERENCES entry_rules (nationality, country) ON DELETE CASCADE
);

-- Las diferencias entre Wikipedia y Passport Index. Se registran para decidir
-- si apagar Passport Index cuando sus datos queden viejos.
CREATE TABLE source_conflicts (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    nationality    text NOT NULL,
    country        text NOT NULL,
    field          text NOT NULL,
    wikipedia      text NOT NULL,
    passport_index text NOT NULL,
    data_date      text NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now()
);

-- El consumo de cada extracción con Claude, para medir el costo real.
CREATE TABLE extraction_usage (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    nationality   text NOT NULL,
    country       text NOT NULL,
    model         text NOT NULL,
    input_tokens  bigint NOT NULL,
    output_tokens bigint NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);

-- La última matriz de Passport Index descargada. Una sola fila: si un día
-- GitHub no responde, se usa esta copia.
CREATE TABLE passport_index_copy (
    id         int PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    -- {"AR-BR": "90", ...}
    entries    jsonb NOT NULL,
    data_date  text NOT NULL,
    fetched_at timestamptz NOT NULL
);
