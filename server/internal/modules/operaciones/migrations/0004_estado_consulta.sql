-- Un viaje nace como consulta: alguien preguntó y todavía no se le cotizó.
-- Antes nacía «cotizado», que afirmaba algo que nadie había hecho. Los viajes
-- que ya existen conservan su estado: no hay forma de saber cuáles se cotizaron.
ALTER TABLE trips DROP CONSTRAINT trips_status_check;
ALTER TABLE trips ADD CONSTRAINT trips_status_check
    CHECK (status IN ('consulta', 'cotizado', 'confirmado', 'en viaje', 'cerrado'));
ALTER TABLE trips ALTER COLUMN status SET DEFAULT 'consulta';
