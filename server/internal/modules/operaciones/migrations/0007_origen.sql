-- De dónde sale el viaje.
--
-- Hasta ahora se asumía que todos salían de Argentina, sin decirlo en ningún
-- lado. Los viajes existentes quedan saliendo de ahí, que es lo que se asumía.
-- El origen no se evalúa como entrada: pesa en la salida de menores, que es una
-- regla argentina, y en la vacuna que se pide a quienes vienen de país de
-- riesgo. Que no esté también en el itinerario lo valida el código, que guarda
-- los dos en la misma transacción.
ALTER TABLE trips
    ADD COLUMN origin_country text NOT NULL DEFAULT 'AR' CHECK (origin_country ~ '^[A-Z]{2}$');
