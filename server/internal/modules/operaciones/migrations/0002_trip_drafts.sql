-- Una operación puede empezar por cualquier dato: pasajero, pago, destino o
-- fecha. Los campos del viaje dejan de ser la puerta de entrada obligatoria y
-- se completan sobre el expediente ya numerado.
ALTER TABLE trips
    ALTER COLUMN destination DROP NOT NULL,
    ALTER COLUMN depart_at DROP NOT NULL,
    ALTER COLUMN return_at DROP NOT NULL;

