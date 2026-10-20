-- Los requisitos de viaje evaluados y las confirmaciones.
--
-- La evaluación cruza las reglas del módulo requisitos con los datos de cada
-- pasajero y se guarda acá, para que los controles se cierren con una consulta
-- como el resto de los derivados. Se reemplaza entera en cada evaluación.

CREATE TABLE trip_requirements (
    trip_id      uuid NOT NULL REFERENCES trips (id) ON DELETE CASCADE,
    position     int  NOT NULL,
    -- NULL: es del viaje (falta el itinerario).
    passenger_id uuid REFERENCES passengers (id) ON DELETE CASCADE,
    -- '': no es de un país (la autorización de menor).
    country      text NOT NULL DEFAULT '',
    role         text NOT NULL DEFAULT '',
    key          text NOT NULL,
    status       text NOT NULL CHECK (status IN ('cumple', 'falta', 'verificar')),
    text         text NOT NULL,
    client_text  text NOT NULL DEFAULT '',
    confirmable  boolean NOT NULL DEFAULT false,
    confirmed    boolean NOT NULL DEFAULT false,
    missing_data boolean NOT NULL DEFAULT false,
    sources      jsonb NOT NULL DEFAULT '[]',
    field_hash   text NOT NULL DEFAULT '',
    PRIMARY KEY (trip_id, position)
);

-- Lo que el sistema no puede saber y una persona sí: si el menor tiene la
-- autorización, si el pasajero tiene la vacuna, qué dice de verdad un campo en
-- «verificar». La confirmación queda atada al valor de la regla con que se hizo
-- (field_hash): si la regla cambia ese campo, deja de valer.
CREATE TABLE requirement_confirmations (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    trip_id      uuid NOT NULL REFERENCES trips (id) ON DELETE CASCADE,
    passenger_id uuid NOT NULL REFERENCES passengers (id) ON DELETE CASCADE,
    country      text NOT NULL DEFAULT '',
    key          text NOT NULL,
    field_hash   text NOT NULL,
    confirmed_by uuid REFERENCES users (id) ON DELETE SET NULL,
    confirmed_at timestamptz NOT NULL DEFAULT now(),
    message_id   uuid REFERENCES messages (id) ON DELETE SET NULL
);

CREATE INDEX requirement_confirmations_trip_idx ON requirement_confirmations (trip_id, passenger_id, country, key);

-- Qué requisitos estaban en falta cuando se mandó el aviso de «Requisitos
-- migratorios». El aviso prueba eso y nada más: un requisito nuevo reabre el
-- control. NULL en las evidencias anteriores, que siguen valiendo como antes.
ALTER TABLE control_evidence ADD COLUMN notified jsonb;

-- «Aviso al pax si requiere visa o vacuna» pasa a ser «Requisitos migratorios»
-- y sigue a los requisitos evaluados. Los viajes abiertos se evalúan al
-- arrancar el servidor; hasta entonces, un control abierto sigue abierto y uno
-- cerrado con aviso sigue cerrado.
UPDATE controls SET derives = 'pasajero.requisitos' WHERE item_id = 'control-visa';
