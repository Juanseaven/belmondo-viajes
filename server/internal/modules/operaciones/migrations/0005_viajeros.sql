-- Los datos que necesita el motor de requisitos de viaje: por qué países pasa el
-- viaje, con qué documento viaja cada pasajero, qué visas tiene y cuándo nació.
--
-- Documentos y permisos cuelgan del pasajero en tablas propias y no como
-- columnas: el día que exista el módulo de clientes, que guardará a cada persona
-- una sola vez, mudarlos será mover dos tablas y no rediseñar.

-- ---------------------------------------------------------------------------
-- El itinerario: una fila por país.
--
-- `trips.destination` sigue siendo el nombre que se muestra («Europa clásica»);
-- esto es lo que se evalúa. Es una lista porque un circuito pasa por varios
-- países y cada uno pide lo suyo. De las escalas alcanza el país.
-- ---------------------------------------------------------------------------
CREATE TABLE trip_countries (
    trip_id   uuid NOT NULL REFERENCES trips (id) ON DELETE CASCADE,
    position  int  NOT NULL,
    country   text NOT NULL CHECK (country ~ '^[A-Z]{2}$'),
    role      text NOT NULL CHECK (role IN ('destino', 'escala')),
    -- Opcionales: sin ellas, la estadía en el país es la del viaje entero.
    arrive_at date,
    leave_at  date,

    PRIMARY KEY (trip_id, position),
    -- Un país se evalúa una vez por viaje.
    CONSTRAINT trip_countries_un_pais UNIQUE (trip_id, country),
    CONSTRAINT trip_countries_fechas CHECK (leave_at IS NULL OR arrive_at IS NULL OR leave_at >= arrive_at)
);

-- ---------------------------------------------------------------------------
-- Los documentos del pasajero.
--
-- La nacionalidad sale del documento y no es un campo del pasajero: un
-- pasaporte italiano dice que es italiano. Quien tiene dos nacionalidades tiene
-- dos documentos y viaja con uno.
-- ---------------------------------------------------------------------------
CREATE TABLE passenger_documents (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    passenger_id uuid NOT NULL REFERENCES passengers (id) ON DELETE CASCADE,
    type         text NOT NULL CHECK (type IN ('DNI', 'Pasaporte')),
    country      text NOT NULL CHECK (country ~ '^[A-Z]{2}$'),
    number       text,
    -- La pide Schengen: el pasaporte tiene que tener menos de diez años.
    issued_at    date,
    expires_at   date,
    created_at   timestamptz NOT NULL DEFAULT now(),

    -- Lo exige la clave compuesta de passengers.travel_document_id.
    CONSTRAINT passenger_documents_del_pasajero UNIQUE (passenger_id, id)
);

CREATE INDEX passenger_documents_passenger_idx ON passenger_documents (passenger_id, created_at);

-- ---------------------------------------------------------------------------
-- Las visas, autorizaciones electrónicas y residencias que ya tiene. Son las
-- que resuelven exenciones como «con visa de EE.UU. vigente alcanza la eTA».
-- ---------------------------------------------------------------------------
CREATE TABLE passenger_permits (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    passenger_id uuid NOT NULL REFERENCES passengers (id) ON DELETE CASCADE,
    kind         text NOT NULL CHECK (kind IN ('visa', 'autorizacion_electronica', 'residencia')),
    country      text NOT NULL CHECK (country ~ '^[A-Z]{2}$'),
    -- Texto libre: «B1/B2», «ESTA».
    label        text,
    valid_from   date,
    expires_at   date,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX passenger_permits_passenger_idx ON passenger_permits (passenger_id, created_at);

-- ---------------------------------------------------------------------------
-- El pasajero gana fecha de nacimiento, residencia y el documento con el que
-- viaja en este viaje.
--
-- La clave compuesta hace que el documento de viaje sea, por construcción, de
-- este pasajero. Al borrarse el documento solo se vacía travel_document_id.
-- ---------------------------------------------------------------------------
ALTER TABLE passengers
    ADD COLUMN birth_date         date,
    ADD COLUMN residence_country  text NOT NULL DEFAULT 'AR' CHECK (residence_country ~ '^[A-Z]{2}$'),
    ADD COLUMN travel_document_id uuid,
    ADD CONSTRAINT passengers_documento_propio
        FOREIGN KEY (id, travel_document_id)
        REFERENCES passenger_documents (passenger_id, id)
        ON DELETE SET NULL (travel_document_id);

-- Los documentos ya cargados eran argentinos: el formulario no preguntaba el
-- país y la agencia vende desde la Argentina. Uno con número y sin tipo se toma
-- como DNI, que es lo que se carga cuando nadie lo aclara.
WITH migrados AS (
    INSERT INTO passenger_documents (passenger_id, type, country, number, expires_at)
    SELECT id, coalesce(document_type, 'DNI'), 'AR', document_number, document_expires_at
      FROM passengers
     WHERE document_type IS NOT NULL OR document_number IS NOT NULL OR document_expires_at IS NOT NULL
    RETURNING id, passenger_id
)
UPDATE passengers p SET travel_document_id = m.id
  FROM migrados m
 WHERE p.id = m.passenger_id;

ALTER TABLE passengers
    DROP COLUMN document_type,
    DROP COLUMN document_number,
    DROP COLUMN document_expires_at;
