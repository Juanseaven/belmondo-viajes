-- Índices que exige la superficie de lectura del panel.
--
-- Prueba de que el reparto de migraciones por módulo funciona: el espejo agrega
-- su 0002 sin coordinarse con el kernel ni con ningún otro módulo.

-- El filtro de líneas suma los sin-revisar de cada línea en cada carga del panel.
-- Sin este índice, ese agregado escanea la tabla entera de conversaciones; con
-- él toca solo las que tienen algo pendiente, que son una fracción diminuta.
--
-- Es un índice parcial por la misma razón que lo hace barato: la enorme mayoría
-- de las conversaciones están en cero, así que no ocupan sitio aquí.
CREATE INDEX conversations_unread_idx
    ON conversations (instance_id)
    WHERE unread_count > 0 AND NOT is_archived;

-- La purga de webhook_events borra por antigüedad y solo lo ya resuelto. El
-- índice de webhook_events_pending_idx cubre lo contrario (pending y failed), así
-- que este es su complemento.
CREATE INDEX webhook_events_purge_idx
    ON webhook_events (received_at)
    WHERE status IN ('processed', 'skipped');
