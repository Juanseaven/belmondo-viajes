-- El detalle de cada requisito y las recomendaciones.
--
-- Cada renglón deja de ser una frase suelta: `text` pasa a ser el título (el
-- nombre del requisito) y `detail` explica qué pide el país, qué tiene el
-- pasajero, qué hay que hacer y, si hay, el trámite. Los renglones guardados se
-- reemplazan en la próxima evaluación de su viaje.
--
-- «aviso» es una recomendación (renovar el pasaporte que vence cerca de la
-- vuelta): no es un faltante y no pesa en los controles.

ALTER TABLE trip_requirements DROP CONSTRAINT trip_requirements_status_check;
ALTER TABLE trip_requirements ADD CONSTRAINT trip_requirements_status_check
    CHECK (status IN ('cumple', 'falta', 'verificar', 'aviso'));

ALTER TABLE trip_requirements ADD COLUMN detail jsonb NOT NULL DEFAULT '{}';
-- La regla del país todavía se está buscando.
ALTER TABLE trip_requirements ADD COLUMN searching boolean NOT NULL DEFAULT false;
