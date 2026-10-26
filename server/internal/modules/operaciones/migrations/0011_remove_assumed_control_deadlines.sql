-- Los vencimientos relativos de la plantilla eran supuestos de producto, no
-- fechas informadas por la agencia. Las fechas reales (por ejemplo, la seña)
-- viven en sus propias tablas y no se modifican.
UPDATE controls SET due_at = NULL WHERE due_at IS NOT NULL;
