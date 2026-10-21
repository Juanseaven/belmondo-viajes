-- El detalle de las páginas oficiales.
--
-- Para cada regla, Claude busca en sitios de gobierno, embajadas y consulados
-- la regla explicada y el trámite, y el código comprueba cada cita contra la
-- página. Va aparte de `fields`: que cambie la redacción no sube la versión ni
-- invalida confirmaciones. La búsqueda no completa campos; solo agrega detalle
-- y puede dejar un campo en «verificar» si contradice a las demás fuentes.

-- Lo que la ficha muestra (domain.Detalles): solo los grupos que valen para
-- los campos actuales. Columna pública, como `fields`.
ALTER TABLE entry_rules ADD COLUMN details jsonb NOT NULL DEFAULT '{}';
-- La búsqueda comprobada (domain.Research), con la huella de los campos contra
-- los que se hizo. Con ella se vuelve a cruzar cada semana sin volver a buscar.
ALTER TABLE entry_rules ADD COLUMN research jsonb;
-- Cuándo toca buscar: una vez por mes; enseguida si un campo cambió; al día
-- siguiente si falló, hasta tres veces seguidas.
ALTER TABLE entry_rules ADD COLUMN research_due_at timestamptz;
ALTER TABLE entry_rules ADD COLUMN research_failures int NOT NULL DEFAULT 0;

-- Las reglas que ya existen se buscan en la próxima vuelta.
UPDATE entry_rules SET research_due_at = now() WHERE status = 'vigente';

ALTER TABLE entry_rule_versions ADD COLUMN details jsonb;

-- Los conflictos ya no son solo con Passport Index: también con las páginas
-- oficiales.
ALTER TABLE source_conflicts RENAME COLUMN passport_index TO other;
ALTER TABLE source_conflicts ADD COLUMN source text NOT NULL DEFAULT 'passport_index';

-- El consumo de las búsquedas: cuántas búsquedas web hizo y para qué fue.
ALTER TABLE extraction_usage ADD COLUMN purpose text NOT NULL DEFAULT 'wikipedia';
ALTER TABLE extraction_usage ADD COLUMN web_searches int NOT NULL DEFAULT 0;
