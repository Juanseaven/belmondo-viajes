-- La bandeja necesita distinguir una conversacion sin revisar de una que espera
-- respuesta. Se materializa la direccion del ultimo mensaje con la misma guarda
-- temporal que preview/type para evitar un JOIN a messages en cada carga.
ALTER TABLE conversations
    ADD COLUMN last_message_direction message_direction;

UPDATE conversations c
   SET last_message_direction = latest.direction
  FROM (
        SELECT DISTINCT ON (conversation_id)
               conversation_id, direction
          FROM messages
         ORDER BY conversation_id, wa_timestamp DESC, id DESC
       ) latest
 WHERE latest.conversation_id = c.id;

CREATE INDEX conversations_unanswered_inbox_idx
    ON conversations (last_message_at DESC NULLS LAST, id DESC)
    WHERE NOT is_archived AND last_message_direction = 'inbound';

CREATE INDEX conversations_unanswered_by_instance_idx
    ON conversations (instance_id, last_message_at DESC NULLS LAST, id DESC)
    WHERE NOT is_archived AND last_message_direction = 'inbound';
