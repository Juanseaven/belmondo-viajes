-- Names saved in WhatsApp belong to one linked line. Two lines may save the
-- same number under different names, so this data cannot live on global contacts.
CREATE TABLE contact_directory (
    instance_id  uuid NOT NULL REFERENCES instances (id) ON DELETE CASCADE,
    wa_jid       text NOT NULL,
    lid_jid      text,
    first_name   text,
    full_name    text,
    push_name    text,
    business_name text,
    updated_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (instance_id, wa_jid)
);

CREATE UNIQUE INDEX contact_directory_lid_idx
    ON contact_directory (instance_id, lid_jid) WHERE lid_jid IS NOT NULL;
CREATE INDEX contact_directory_name_trgm_idx ON contact_directory
    USING gin ((coalesce(full_name, first_name, business_name, push_name, '')) gin_trgm_ops);

-- Older ingestion treated the numeric prefix of a group JID as a telephone.
UPDATE contacts SET phone_number = NULL WHERE wa_jid LIKE '%@g.us';

-- Seed line-local push names and LID mappings collected by HistorySync.
WITH directory AS (
    SELECT DISTINCT ON (instance.id, entry->>'ID')
           instance.id AS instance_id,
           entry->>'ID' AS jid,
           nullif(entry->>'pushname', '') AS push_name,
           event.received_at
      FROM webhook_events event
      JOIN instances instance ON instance.evolution_name = event.instance_name
      CROSS JOIN LATERAL jsonb_array_elements(
          CASE WHEN jsonb_typeof(event.payload->'data'->'Data'->'pushnames') = 'array'
               THEN event.payload->'data'->'Data'->'pushnames'
               ELSE '[]'::jsonb END
      ) entry
     WHERE event.event = 'HistorySync'
       AND nullif(entry->>'ID', '') IS NOT NULL
     ORDER BY instance.id, entry->>'ID', event.received_at DESC
), mappings AS (
    SELECT DISTINCT ON (instance.id, entry->>'pnJID')
           instance.id AS instance_id,
           entry->>'pnJID' AS phone_jid,
           nullif(entry->>'lidJID', '') AS lid_jid
      FROM webhook_events event
      JOIN instances instance ON instance.evolution_name = event.instance_name
      CROSS JOIN LATERAL jsonb_array_elements(
          CASE WHEN jsonb_typeof(event.payload->'data'->'Data'->'phoneNumberToLidMappings') = 'array'
               THEN event.payload->'data'->'Data'->'phoneNumberToLidMappings'
               ELSE '[]'::jsonb END
      ) entry
     WHERE event.event = 'HistorySync'
       AND nullif(entry->>'pnJID', '') IS NOT NULL
     ORDER BY instance.id, entry->>'pnJID', event.received_at DESC
)
INSERT INTO contact_directory (instance_id, wa_jid, lid_jid, push_name)
SELECT coalesce(directory.instance_id, mappings.instance_id),
       coalesce(directory.jid, mappings.phone_jid),
       mappings.lid_jid,
       directory.push_name
  FROM directory
  FULL JOIN mappings
    ON mappings.instance_id = directory.instance_id
   AND mappings.phone_jid = directory.jid
ON CONFLICT (instance_id, wa_jid) DO UPDATE
SET lid_jid = coalesce(excluded.lid_jid, contact_directory.lid_jid),
    push_name = coalesce(excluded.push_name, contact_directory.push_name),
    updated_at = now();

-- Restore group subjects from the raw history already retained for forensics.
WITH group_names AS (
    SELECT DISTINCT ON (instance.id, conversation->>'ID')
           instance.id AS instance_id,
           conversation->>'ID' AS group_jid,
           nullif(conversation->>'name', '') AS group_name
      FROM webhook_events event
      JOIN instances instance ON instance.evolution_name = event.instance_name
      CROSS JOIN LATERAL jsonb_array_elements(
          CASE WHEN jsonb_typeof(event.payload->'data'->'Data'->'conversations') = 'array'
               THEN event.payload->'data'->'Data'->'conversations'
               ELSE '[]'::jsonb END
      ) conversation
     WHERE event.event = 'HistorySync'
       AND conversation->>'ID' LIKE '%@g.us'
       AND nullif(conversation->>'name', '') IS NOT NULL
     ORDER BY instance.id, conversation->>'ID', event.received_at DESC
)
UPDATE contacts contact
   SET push_name = names.group_name
  FROM conversations conversation, group_names names
 WHERE conversation.contact_id = contact.id
   AND conversation.instance_id = names.instance_id
   AND conversation.remote_jid = names.group_jid;

-- Recover participant attribution from live group messages received before the
-- sender columns were introduced. Prefer the phone JID over an opaque LID.
WITH live_senders AS (
    SELECT DISTINCT ON (instance.id, payload->'data'->'Info'->>'ID')
           instance.id AS instance_id,
           payload->'data'->'Info'->>'ID' AS wa_message_id,
           CASE
             WHEN payload->'data'->'Info'->>'Sender' LIKE '%@s.whatsapp.net'
               THEN payload->'data'->'Info'->>'Sender'
             WHEN payload->'data'->'Info'->>'SenderAlt' LIKE '%@s.whatsapp.net'
               THEN payload->'data'->'Info'->>'SenderAlt'
             ELSE coalesce(payload->'data'->'Info'->>'Sender', payload->'data'->'Info'->>'SenderAlt')
           END AS sender_jid,
           nullif(payload->'data'->'Info'->>'PushName', '') AS sender_name
      FROM webhook_events event
      JOIN instances instance ON instance.evolution_name = event.instance_name
     WHERE event.event IN ('Message', 'SendMessage')
       AND coalesce((payload->'data'->'Info'->>'IsGroup')::boolean, false)
       AND NOT coalesce((payload->'data'->'Info'->>'IsFromMe')::boolean, false)
       AND nullif(payload->'data'->'Info'->>'ID', '') IS NOT NULL
     ORDER BY instance.id, payload->'data'->'Info'->>'ID', event.received_at DESC
)
UPDATE messages message
   SET sender_jid = coalesce(message.sender_jid, sender.sender_jid),
       sender_name = coalesce(message.sender_name, sender.sender_name)
  FROM live_senders sender
 WHERE message.instance_id = sender.instance_id
   AND message.wa_message_id = sender.wa_message_id;
