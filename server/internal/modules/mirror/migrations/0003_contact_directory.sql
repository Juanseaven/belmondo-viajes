-- Preserve the identity information that WhatsApp sends outside message data.
ALTER TABLE contacts ADD COLUMN lid_jid text;
CREATE UNIQUE INDEX contacts_lid_jid_idx
    ON contacts (lid_jid) WHERE lid_jid IS NOT NULL;

ALTER TABLE messages ADD COLUMN sender_jid text;
ALTER TABLE messages ADD COLUMN sender_name text;
CREATE INDEX messages_sender_jid_idx
    ON messages (sender_jid) WHERE sender_jid IS NOT NULL;

-- Recover contact names already received in HistorySync before this migration.
WITH directory AS (
    SELECT DISTINCT ON (entry->>'ID')
           entry->>'ID' AS jid,
           nullif(entry->>'pushname', '') AS push_name
      FROM webhook_events event
      CROSS JOIN LATERAL jsonb_array_elements(
          CASE WHEN jsonb_typeof(event.payload->'data'->'Data'->'pushnames') = 'array'
               THEN event.payload->'data'->'Data'->'pushnames'
               ELSE '[]'::jsonb END
      ) entry
     WHERE event.event = 'HistorySync'
       AND nullif(entry->>'ID', '') IS NOT NULL
     ORDER BY entry->>'ID', event.received_at DESC
)
INSERT INTO contacts (wa_jid, phone_number, push_name)
SELECT jid,
       CASE WHEN jid LIKE '%@s.whatsapp.net'
            THEN replace(jid, '@s.whatsapp.net', '') END,
       push_name
  FROM directory
ON CONFLICT (wa_jid) DO UPDATE
SET push_name = coalesce(excluded.push_name, contacts.push_name),
    phone_number = coalesce(contacts.phone_number, excluded.phone_number);

-- Link opaque LID participant ids to their phone contacts.
WITH mappings AS (
    SELECT DISTINCT ON (entry->>'pnJID')
           entry->>'pnJID' AS phone_jid,
           nullif(entry->>'lidJID', '') AS lid_jid
      FROM webhook_events event
      CROSS JOIN LATERAL jsonb_array_elements(
          CASE WHEN jsonb_typeof(event.payload->'data'->'Data'->'phoneNumberToLidMappings') = 'array'
               THEN event.payload->'data'->'Data'->'phoneNumberToLidMappings'
               ELSE '[]'::jsonb END
      ) entry
     WHERE event.event = 'HistorySync'
       AND nullif(entry->>'pnJID', '') IS NOT NULL
     ORDER BY entry->>'pnJID', event.received_at DESC
)
INSERT INTO contacts (wa_jid, phone_number, lid_jid)
SELECT phone_jid, replace(phone_jid, '@s.whatsapp.net', ''), lid_jid
  FROM mappings
ON CONFLICT (wa_jid) DO UPDATE
SET lid_jid = coalesce(excluded.lid_jid, contacts.lid_jid),
    phone_number = coalesce(contacts.phone_number, excluded.phone_number);

-- Recover the participant attached to group messages already imported.
WITH history_messages AS (
    SELECT DISTINCT ON (event.instance_name,
                        coalesce(message->'key'->>'ID', message->'key'->>'id'))
           event.instance_name,
           coalesce(message->'key'->>'ID', message->'key'->>'id') AS wa_message_id,
           nullif(coalesce(message->>'participant', message->'key'->>'participant'), '') AS sender_jid,
           nullif(message->>'pushName', '') AS sender_name
      FROM webhook_events event
      CROSS JOIN LATERAL jsonb_array_elements(
          CASE WHEN jsonb_typeof(event.payload->'data'->'Data'->'conversations') = 'array'
               THEN event.payload->'data'->'Data'->'conversations'
               ELSE '[]'::jsonb END
      ) conversation
      CROSS JOIN LATERAL jsonb_array_elements(
          CASE WHEN jsonb_typeof(conversation->'messages') = 'array'
               THEN conversation->'messages'
               ELSE '[]'::jsonb END
      ) wrapper
      CROSS JOIN LATERAL (SELECT wrapper->'message' AS message) parsed
     WHERE event.event = 'HistorySync'
       AND nullif(coalesce(message->>'participant', message->'key'->>'participant'), '') IS NOT NULL
       AND nullif(coalesce(message->'key'->>'ID', message->'key'->>'id'), '') IS NOT NULL
     ORDER BY event.instance_name,
              coalesce(message->'key'->>'ID', message->'key'->>'id'),
              event.received_at DESC
)
UPDATE messages stored
   SET sender_jid = history.sender_jid,
       sender_name = coalesce(history.sender_name, stored.sender_name)
  FROM instances instance, history_messages history
 WHERE stored.instance_id = instance.id
   AND instance.evolution_name = history.instance_name
   AND stored.wa_message_id = history.wa_message_id;
