-- El consumo de la caché. input_tokens no incluye lo que se leyó de la caché
-- (cuesta un 5% de la entrada) ni lo que se escribió (un 25% más): sin estas
-- columnas, el costo de una búsqueda no se puede calcular.
ALTER TABLE extraction_usage ADD COLUMN cache_read_tokens bigint NOT NULL DEFAULT 0;
ALTER TABLE extraction_usage ADD COLUMN cache_write_tokens bigint NOT NULL DEFAULT 0;
