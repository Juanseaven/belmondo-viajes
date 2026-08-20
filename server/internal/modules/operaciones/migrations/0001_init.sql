-- Módulo operaciones: la base de clientes y viajes de la agencia.
--
-- Reemplaza dos planillas en papel —la de carga de reserva y la de control— sin
-- convertirlas en una lista de casilleros. La regla que ordena todo vive en
-- controls.closing: cuando el dato alcanza para cumplir el control, el sistema
-- lo cierra solo; cuando la acción se puede ejecutar desde el panel, se cierra
-- al ejecutarla y queda la prueba; y solo lo que de verdad exige comparar contra
-- el sistema del operador se confirma a mano.
--
-- Este módulo LEE tres tablas públicas del módulo espejo —contacts,
-- conversations y messages— y tiene claves foráneas hacia ellas. No escribe
-- ninguna: las invariantes que el espejo mantiene en código no se saltean desde
-- afuera. Por eso el puente cliente-contacto vive acá y no como una columna en
-- contacts, y por eso desinstalar este módulo deja el espejo intacto.

-- ---------------------------------------------------------------------------
-- El cliente de la agencia.
--
-- No es el contacto de WhatsApp. Un contacto es quien escribió; un cliente es a
-- quien se le vende. El mismo cliente escribe desde su número y desde el de la
-- mujer, «Familia Rossi» es un grupo y no un cliente, y quien compra por
-- teléfono no tiene contacto alguno.
-- ---------------------------------------------------------------------------
CREATE TABLE customers (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name            text NOT NULL,
    -- Lo que WhatsApp no da. Las planillas los piden de forma explícita:
    -- «tel y mail del pax cargados en reserva».
    email           text,
    phone_number    text,
    document_type   text,
    document_number text,
    customer_since  date NOT NULL DEFAULT CURRENT_DATE,
    created_by      uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE TRIGGER customers_touch BEFORE UPDATE ON customers
    FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

-- El puente hacia los contactos del espejo.
--
-- contact_id es UNIQUE: un contacto pertenece a un solo cliente, pero un cliente
-- puede tener varios contactos. Eso es lo que hace que el mismo cliente que
-- escribe desde dos números sea una sola historia.
CREATE TABLE customer_contacts (
    customer_id uuid NOT NULL REFERENCES customers (id) ON DELETE CASCADE,
    contact_id  uuid NOT NULL UNIQUE REFERENCES contacts (id) ON DELETE CASCADE,
    linked_by   uuid REFERENCES users (id) ON DELETE SET NULL,
    linked_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (customer_id, contact_id)
);

-- ---------------------------------------------------------------------------
-- El viaje: el expediente.
--
-- Se llama «viaje» y no «operación» porque es la palabra que usa la agencia, y
-- el código habla el mismo idioma que la pantalla para que nadie traduzca.
--
-- No hay columna de «viaje activo»: un cliente puede tener legítimamente dos a
-- la vez, uno confirmado y otro cotizándose. El banner muestra el más reciente
-- sin cerrar. Una columna is_active sería un dato más que puede mentir.
-- ---------------------------------------------------------------------------
CREATE TABLE trips (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    -- El número que la agencia dice en voz alta. Correlativo y estable.
    number         bigint GENERATED ALWAYS AS IDENTITY UNIQUE,
    customer_id    uuid NOT NULL REFERENCES customers (id) ON DELETE CASCADE,
    status         text NOT NULL DEFAULT 'cotizado'
                       CHECK (status IN ('cotizado', 'confirmado', 'en viaje', 'cerrado')),
    destination    text NOT NULL,
    depart_at      date NOT NULL,
    return_at      date NOT NULL,
    -- El vendedor es una persona, no la línea de WhatsApp por la que se habló.
    seller_user_id uuid REFERENCES users (id) ON DELETE SET NULL,
    created_by     uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT trips_fechas_coherentes CHECK (return_at >= depart_at)
);

CREATE INDEX trips_customer_idx ON trips (customer_id, created_at DESC);

CREATE TRIGGER trips_touch BEFORE UPDATE ON trips
    FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

-- ---------------------------------------------------------------------------
-- Los pasajeros. La lista de personas que reemplaza al conteo «2 ad + 1 men».
--
-- El nombre se guarda tal como figura en el documento: el orden de nombres y
-- apellidos es uno de los controles de la planilla, y un nombre mal escrito en
-- un ticket aéreo no se corrige.
-- ---------------------------------------------------------------------------
CREATE TABLE passengers (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    trip_id             uuid NOT NULL REFERENCES trips (id) ON DELETE CASCADE,
    first_name          text NOT NULL,
    last_name           text NOT NULL,
    kind                text NOT NULL DEFAULT 'adulto' CHECK (kind IN ('adulto', 'menor')),
    is_holder           boolean NOT NULL DEFAULT false,
    document_type       text CHECK (document_type IN ('DNI', 'Pasaporte')),
    document_number     text,
    -- Con esta fecha, el control del documento vencido se dispara solo en vez de
    -- depender de que alguien lo mire.
    document_expires_at date,
    created_by          uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX passengers_trip_idx ON passengers (trip_id);

-- Un solo titular por viaje: la planilla controla «nombre y apellido del titular
-- de la reserva», en singular.
CREATE UNIQUE INDEX passengers_un_solo_titular ON passengers (trip_id) WHERE is_holder;

CREATE TRIGGER passengers_touch BEFORE UPDATE ON passengers
    FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

-- ---------------------------------------------------------------------------
-- El pago. Una fila por viaje.
--
-- «Quién me debe la seña y para cuándo» es la pregunta que la agencia se hace
-- todas las mañanas, y es la mitad del expediente que más rápido le cambia el
-- día. Registrar cada cobro por separado será otra tabla el día que haga falta.
-- ---------------------------------------------------------------------------
CREATE TABLE payments (
    trip_id            uuid PRIMARY KEY REFERENCES trips (id) ON DELETE CASCADE,
    total_usd          numeric(12, 2),
    -- Cambio del día, como lo pide la planilla de carga.
    rate               numeric(12, 4),
    total_ars          numeric(14, 2),
    method             text,
    deposit_usd        numeric(12, 2),
    deposit_due_at     date,
    deposit_paid_at    timestamptz,
    non_refundable_usd numeric(12, 2),
    updated_by         uuid REFERENCES users (id) ON DELETE SET NULL,
    updated_at         timestamptz NOT NULL DEFAULT now()
);

CREATE TRIGGER payments_touch BEFORE UPDATE ON payments
    FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

-- ---------------------------------------------------------------------------
-- Los controles: la copia que cada viaje se lleva de la plantilla al nacer.
--
-- La plantilla no es una tabla. Vive en Go como constante porque es el proceso
-- de la empresa y no un dato de cada viaje; que cada viaje guarde su copia hace
-- que cambiar el proceso no reescriba la historia de los viajes ya abiertos.
--
-- item_id es la clave del renglón en esa plantilla. closing, stage y critical se
-- copian junto con él para que una consulta pueda ordenar la bandeja sin
-- consultar la plantilla, y para que un renglón que mañana cambia de forma de
-- cierre no altere los viajes que ya nacieron con la anterior.
-- ---------------------------------------------------------------------------
CREATE TABLE controls (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    trip_id    uuid NOT NULL REFERENCES trips (id) ON DELETE CASCADE,
    item_id    text NOT NULL,
    stage      text NOT NULL CHECK (stage IN ('carga', 'control')),
    closing    text NOT NULL CHECK (closing IN ('derivado', 'accion', 'manual')),
    critical   boolean NOT NULL DEFAULT false,
    status     text NOT NULL DEFAULT 'abierto' CHECK (status IN ('abierto', 'cerrado')),
    -- NULL en los derivados: un control que el sistema cierra solo no tiene a
    -- quién apurar, y darle fecha lo pondría a competir en el chip de la bandeja
    -- con los que sí necesitan que alguien haga algo.
    due_at     date,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT controls_uno_por_renglon UNIQUE (trip_id, item_id)
);

-- El chip de la bandeja: el control crítico abierto más próximo a vencer.
CREATE INDEX controls_pendientes_idx ON controls (trip_id, critical DESC, due_at)
    WHERE status = 'abierto';

CREATE TRIGGER controls_touch BEFORE UPDATE ON controls
    FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

-- ---------------------------------------------------------------------------
-- La evidencia: quién cerró un control, cuándo, y con qué.
--
-- Va en tabla aparte y no como columnas en controls para que reabrir un control
-- no borre la historia de que alguna vez se cerró. message_id apunta a la tabla
-- pública del espejo: es la diferencia entre un registro que afirma que se avisó
-- de la visa y el mensaje que lo prueba.
-- ---------------------------------------------------------------------------
CREATE TABLE control_evidence (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    control_id uuid NOT NULL REFERENCES controls (id) ON DELETE CASCADE,
    closed_by  uuid REFERENCES users (id) ON DELETE SET NULL,
    closed_at  timestamptz NOT NULL DEFAULT now(),
    message_id uuid REFERENCES messages (id) ON DELETE SET NULL
);

CREATE INDEX control_evidence_control_idx ON control_evidence (control_id, closed_at DESC);
