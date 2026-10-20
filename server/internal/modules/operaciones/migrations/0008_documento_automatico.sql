-- El documento de viaje lo elige el sistema, salvo que una persona lo elija a
-- mano.
--
-- En cada evaluación se prueba cada documento del pasajero contra los países
-- del itinerario y se queda el que mejor cumple. travel_document_manual marca
-- que alguien eligió «viaja con este»: entonces no se toca. Los elegidos hasta
-- hoy eran el primero que se cargó, no una decisión: quedan automáticos.
ALTER TABLE passengers ADD COLUMN travel_document_manual boolean NOT NULL DEFAULT false;

-- Ningún plazo queda antes del alta del viaje: el que caía antes, por una
-- salida cercana, pasa al día del alta. Es la misma regla que aplica el código
-- al calcular los plazos de ahora en más.
UPDATE controls c
   SET due_at = t.created_at::date
  FROM trips t
 WHERE c.trip_id = t.id AND c.due_at < t.created_at::date;
