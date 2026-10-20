-- Evolution Go replays messages missed while the mirror is offline as regular
-- Message events. Before linked-device read receipts were handled, every replayed
-- inbound message increased unread_count even when it had already been read in
-- WhatsApp. Rebuild only conversations for which the forensic webhook log has a
-- read made by this account; conversations without such evidence keep their
-- current counter.
WITH latest_reads AS (
    SELECT i.id AS instance_id,
           e.payload #>> '{data,Chat}' AS chat_jid,
           max((e.payload #>> '{data,Timestamp}')::timestamptz) AS read_at
      FROM webhook_events e
      JOIN instances i ON i.evolution_name = e.instance_name
     WHERE e.event = 'Receipt'
       AND coalesce((e.payload #>> '{data,IsFromMe}')::boolean, false)
       AND lower(e.payload #>> '{data,Type}') IN ('read', 'read-self')
       AND nullif(e.payload #>> '{data,Timestamp}', '') IS NOT NULL
     GROUP BY i.id, e.payload #>> '{data,Chat}'
), conversation_reads AS (
    SELECT c.id AS conversation_id, max(r.read_at) AS read_at
      FROM conversations c
      JOIN contacts ct ON ct.id = c.contact_id
      JOIN latest_reads r
        ON r.instance_id = c.instance_id
       AND r.chat_jid IN (c.remote_jid, ct.wa_jid, ct.lid_jid)
     GROUP BY c.id
), reconciled AS (
    SELECT cr.conversation_id,
           count(m.id) FILTER (
               WHERE m.direction = 'inbound' AND m.wa_timestamp > cr.read_at
           )::int AS unread_count
      FROM conversation_reads cr
      LEFT JOIN messages m ON m.conversation_id = cr.conversation_id
     GROUP BY cr.conversation_id
)
UPDATE conversations c
   SET unread_count = r.unread_count
  FROM reconciled r
 WHERE c.id = r.conversation_id
   AND c.unread_count <> r.unread_count;
