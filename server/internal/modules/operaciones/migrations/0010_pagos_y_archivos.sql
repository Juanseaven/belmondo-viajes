-- Los pagos registrados y los archivos favoritos.

-- ---------------------------------------------------------------------------
-- Los pagos: cada plata que entró, una fila.
--
-- `payments` sigue siendo la cotización del viaje (una fila por viaje). Su
-- comentario lo anticipaba: «Registrar cada cobro por separado será otra tabla
-- el día que haga falta». La barra de «pagado contra total» suma los
-- equivalentes en dólares de estas filas.
-- ---------------------------------------------------------------------------
CREATE TABLE trip_payments (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    trip_id    uuid NOT NULL REFERENCES trips (id) ON DELETE CASCADE,
    amount     numeric(14, 2) NOT NULL CHECK (amount > 0),
    currency   text NOT NULL CHECK (currency IN ('USD', 'ARS')),
    -- El cambio de ese pago. Solo en pesos: el equivalente en dólares sale de él.
    rate       numeric(12, 4) CHECK (rate > 0),
    amount_usd numeric(12, 2) NOT NULL CHECK (amount_usd > 0),
    paid_at    date NOT NULL,
    method     text,
    note       text,
    created_by uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT trip_payments_cambio_en_pesos CHECK ((currency = 'ARS') = (rate IS NOT NULL))
);

CREATE INDEX trip_payments_trip_idx ON trip_payments (trip_id, paid_at, created_at);

-- La seña ya no se marca a mano: se da por cobrada cuando los pagos llegan a
-- su monto. Una seña que figuraba cobrada sin monto no se puede convertir en un
-- pago —no se sabe cuánto se pagó— y conserva su fecha con esta marca, para que
-- no vuelva a pedirse una seña que ya se cobró.
ALTER TABLE payments ADD COLUMN deposit_paid_before boolean NOT NULL DEFAULT false;

UPDATE payments SET deposit_paid_before = true
 WHERE deposit_paid_at IS NOT NULL AND deposit_usd IS NULL;

-- Las señas cobradas con monto pasan a ser el primer pago del viaje.
INSERT INTO trip_payments (trip_id, amount, currency, amount_usd, paid_at, method, note, created_by, created_at)
SELECT trip_id, deposit_usd, 'USD', deposit_usd,
       (deposit_paid_at AT TIME ZONE 'America/Argentina/Buenos_Aires')::date,
       method, 'Seña', updated_by, deposit_paid_at
  FROM payments
 WHERE deposit_paid_at IS NOT NULL AND deposit_usd IS NOT NULL AND deposit_usd > 0;

-- El total en pesos se calcula: total en dólares por el cambio del día.
UPDATE payments
   SET total_ars = CASE WHEN total_usd IS NOT NULL AND rate IS NOT NULL
                        THEN round(total_usd * rate) END;

-- ---------------------------------------------------------------------------
-- Los archivos favoritos: la estrella es de la agencia, no de cada persona.
--
-- El archivo vive en el espejo (messages y media_assets); acá solo se guarda
-- que alguien lo marcó. Borrar el mensaje borra la marca.
-- ---------------------------------------------------------------------------
CREATE TABLE file_favorites (
    message_id uuid PRIMARY KEY REFERENCES messages (id) ON DELETE CASCADE,
    marked_by  uuid REFERENCES users (id) ON DELETE SET NULL,
    marked_at  timestamptz NOT NULL DEFAULT now()
);
