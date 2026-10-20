-- WhatsApp may identify the same private chat with two JIDs:
--   * the stable phone JID: 54911...@s.whatsapp.net
--   * an opaque LID:         37035...@lid
-- Incoming messages normally use the former while messages written from the
-- linked phone can use the latter. contacts.lid_jid already records that both
-- identities are the same person; consolidate conversations created before the
-- write path began honoring that mapping.

-- Live inbound events also carry the mapping in SenderAlt. Keep this recovery
-- independent from HistorySync so a recent contact can be repaired immediately.
WITH live_mappings AS (
    SELECT DISTINCT ON (payload #>> '{data,Info,Chat}')
           payload #>> '{data,Info,Chat}' AS phone_jid,
           payload #>> '{data,Info,SenderAlt}' AS lid_jid,
           nullif(payload #>> '{data,Info,PushName}', '') AS push_name
      FROM webhook_events
     WHERE event = 'Message'
       AND payload #>> '{data,Info,Chat}' LIKE '%@s.whatsapp.net'
       AND payload #>> '{data,Info,SenderAlt}' LIKE '%@lid'
       AND coalesce(payload #>> '{data,Info,IsGroup}', 'false') = 'false'
     ORDER BY payload #>> '{data,Info,Chat}', received_at DESC
)
INSERT INTO contacts (wa_jid, phone_number, push_name, lid_jid)
SELECT phone_jid,
       '+' || split_part(phone_jid, '@', 1),
       push_name,
       lid_jid
  FROM live_mappings
ON CONFLICT (wa_jid) DO UPDATE SET
    phone_number = coalesce(contacts.phone_number, excluded.phone_number),
    push_name = coalesce(contacts.push_name, excluded.push_name),
    lid_jid = coalesce(contacts.lid_jid, excluded.lid_jid);

CREATE TEMP TABLE mirror_lid_contact_merges ON COMMIT DROP AS
SELECT phone.id AS canonical_contact_id,
       phone.wa_jid AS phone_jid,
       lid.id AS alias_contact_id
  FROM contacts phone
  JOIN contacts lid ON lid.wa_jid = phone.lid_jid
 WHERE phone.wa_jid LIKE '%@s.whatsapp.net'
   AND lid.wa_jid LIKE '%@lid'
   AND phone.id <> lid.id;

-- Preserve any useful metadata from the alias. The alias row stays in place
-- because other modules may reference contacts independently of the inbox.
UPDATE contacts canonical
   SET display_name = coalesce(canonical.display_name, alias.display_name),
       push_name = coalesce(canonical.push_name, alias.push_name),
       profile_pic_url = coalesce(canonical.profile_pic_url, alias.profile_pic_url),
       notes = coalesce(canonical.notes, alias.notes),
       is_blocked = canonical.is_blocked OR alias.is_blocked
  FROM mirror_lid_contact_merges mapping
  JOIN contacts alias ON alias.id = mapping.alias_contact_id
 WHERE canonical.id = mapping.canonical_contact_id;

CREATE TEMP TABLE mirror_lid_conversation_merges ON COMMIT DROP AS
SELECT mapping.canonical_contact_id,
       mapping.phone_jid,
       alias.id AS alias_conversation_id,
       canonical.id AS canonical_conversation_id
  FROM mirror_lid_contact_merges mapping
  JOIN conversations alias ON alias.contact_id = mapping.alias_contact_id
  LEFT JOIN conversations canonical
    ON canonical.instance_id = alias.instance_id
   AND canonical.contact_id = mapping.canonical_contact_id;

-- When both conversations exist, move every dependent row before deleting the
-- empty LID conversation. Message IDs and timestamps are unchanged.
UPDATE messages message
   SET conversation_id = mapping.canonical_conversation_id,
       contact_id = mapping.canonical_contact_id
  FROM mirror_lid_conversation_merges mapping
 WHERE mapping.canonical_conversation_id IS NOT NULL
   AND message.conversation_id = mapping.alias_conversation_id;

UPDATE outbound_messages outbound
   SET conversation_id = mapping.canonical_conversation_id
  FROM mirror_lid_conversation_merges mapping
 WHERE mapping.canonical_conversation_id IS NOT NULL
   AND outbound.conversation_id = mapping.alias_conversation_id;

UPDATE conversations canonical
   SET remote_jid = mapping.phone_jid,
       last_message_preview = CASE
           WHEN canonical.last_message_at IS NULL
             OR (alias.last_message_at IS NOT NULL AND alias.last_message_at >= canonical.last_message_at)
           THEN alias.last_message_preview ELSE canonical.last_message_preview END,
       last_message_type = CASE
           WHEN canonical.last_message_at IS NULL
             OR (alias.last_message_at IS NOT NULL AND alias.last_message_at >= canonical.last_message_at)
           THEN alias.last_message_type ELSE canonical.last_message_type END,
       last_message_direction = CASE
           WHEN canonical.last_message_at IS NULL
             OR (alias.last_message_at IS NOT NULL AND alias.last_message_at >= canonical.last_message_at)
           THEN alias.last_message_direction ELSE canonical.last_message_direction END,
       last_message_at = CASE
           WHEN canonical.last_message_at IS NULL
             OR (alias.last_message_at IS NOT NULL AND alias.last_message_at >= canonical.last_message_at)
           THEN alias.last_message_at ELSE canonical.last_message_at END,
       unread_count = canonical.unread_count + alias.unread_count,
       is_archived = canonical.is_archived AND alias.is_archived,
       created_at = least(canonical.created_at, alias.created_at)
  FROM mirror_lid_conversation_merges mapping
  JOIN conversations alias ON alias.id = mapping.alias_conversation_id
 WHERE mapping.canonical_conversation_id IS NOT NULL
   AND canonical.id = mapping.canonical_conversation_id;

DELETE FROM conversations alias
 USING mirror_lid_conversation_merges mapping
 WHERE mapping.canonical_conversation_id IS NOT NULL
   AND alias.id = mapping.alias_conversation_id;

-- If only the LID conversation existed, keep its ID and simply retarget it to
-- the canonical contact. This preserves URLs and any external references.
UPDATE messages message
   SET contact_id = mapping.canonical_contact_id
  FROM mirror_lid_conversation_merges mapping
 WHERE mapping.canonical_conversation_id IS NULL
   AND message.conversation_id = mapping.alias_conversation_id;

UPDATE conversations conversation
   SET contact_id = mapping.canonical_contact_id,
       remote_jid = mapping.phone_jid
  FROM mirror_lid_conversation_merges mapping
 WHERE mapping.canonical_conversation_id IS NULL
   AND conversation.id = mapping.alias_conversation_id;

