-- El control del vencimiento de DNI o pasaporte pasa a mirar los documentos
-- cargados: se cierra solo si todos los pasajeros tienen un documento que vence
-- después del regreso. Antes era solo «por acción», y un pasaporte vigente
-- seguía dando la alerta hasta que alguien mandara un aviso que no hacía falta.
--
-- Los viajes ya abiertos guardan su copia del renglón: se les suma el dato que
-- lo cumple y se recalcula una vez, con la misma regla que aplica
-- refreshDerived en cada escritura.
UPDATE controls
   SET derives = 'pasajero.documentos_vigentes'
 WHERE item_id = 'control-vto-doc';

UPDATE controls c
   SET status = 'cerrado'
  FROM trips t
 WHERE c.trip_id = t.id
   AND c.item_id = 'control-vto-doc'
   AND c.status = 'abierto'
   AND EXISTS (SELECT 1 FROM passengers p WHERE p.trip_id = t.id)
   AND NOT EXISTS (SELECT 1 FROM passengers p
                    WHERE p.trip_id = t.id
                      AND (p.document_expires_at IS NULL
                           OR p.document_expires_at < coalesce(t.return_at, t.depart_at)));
