package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
)

// execer es lo mínimo que necesita el recálculo. Lo cumplen tanto el pool como
// una transacción en curso, para que el alta lo haga dentro de la suya sin una
// segunda ida a la base.
type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// RefreshDerived recalcula los controles que se cierran solos.
//
// Se llama después de cada escritura que pueda cambiar un dato del expediente.
// Es idempotente y barata: una sola consulta que compara el estado guardado
// contra la realidad y escribe únicamente las filas que cambian.
//
// Que también REABRA es la mitad que importa. Un control que se cierra y nunca
// vuelve a mirarse es la misma mentira que un tilde, con más pasos: si se borra
// el único pasajero, «Nombres completos» tiene que volver a estar pendiente.
func (s *Store) RefreshDerived(ctx context.Context, tripID string) error {
	return refreshDerived(ctx, s.pool, tripID)
}

func refreshDerived(ctx context.Context, q execer, tripID string) error {
	if _, err := q.Exec(ctx, sqlRefreshDerived, tripID); err != nil {
		return fmt.Errorf("recalculando los controles derivados: %w", err)
	}
	return nil
}

// El mapeo es por DATO y no por renglón: la columna `derives` dice qué hace
// falta, no qué fila es. Renombrar un renglón de la plantilla no rompe esto.
//
// Un derivado sigue al dato en las dos direcciones. Un control por acción con
// `derives` se cierra cuando el dato está, y se reabre cuando deja de estar
// solo si el cierre fue del dato: si lo cerró un aviso, ese mensaje prueba que
// se hizo lo que el control pedía, y el dato no lo deshace.
//
// Los documentos «llegan al regreso» si el documento con el que viaja cada
// pasajero vence después de la fecha de regreso, o de la de salida mientras no
// haya regreso, y además cumple la vigencia y la antigüedad que piden los
// países del itinerario. Donde la regla no sabe la vigencia, queda el criterio
// del regreso.
//
// Los requisitos migratorios están al día si todos los evaluados cumplen,
// salvo vigencia y antigüedad, que son del control del documento.
const sqlRefreshDerived = `
WITH datos AS (
    SELECT t.id AS trip_id,
           coalesce(btrim(t.destination), '') <> ''                       AS destino,
	       t.depart_at IS NOT NULL                                          AS salida,
	       t.return_at IS NOT NULL                                          AS regreso,
           EXISTS (SELECT 1 FROM passengers p
                    WHERE p.trip_id = t.id AND btrim(p.first_name) <> '') AS nombre,
           EXISTS (SELECT 1 FROM passengers p
                    WHERE p.trip_id = t.id AND btrim(p.last_name) <> '')  AS apellido,
           EXISTS (SELECT 1 FROM passengers p
                    WHERE p.trip_id = t.id AND p.is_holder)               AS titular,
           (EXISTS (SELECT 1 FROM passengers p WHERE p.trip_id = t.id)
            AND NOT EXISTS (SELECT 1 FROM passengers p
                              LEFT JOIN passenger_documents d ON d.id = p.travel_document_id
                             WHERE p.trip_id = t.id
                               AND (d.expires_at IS NULL
                                    OR d.expires_at < coalesce(t.return_at, t.depart_at)))
            AND NOT EXISTS (SELECT 1 FROM trip_requirements r
                             WHERE r.trip_id = t.id AND r.key IN ('vigencia', 'antiguedad')
                               AND r.status = 'falta'))                   AS documentos,
           (EXISTS (SELECT 1 FROM trip_requirements r
                     WHERE r.trip_id = t.id AND r.key NOT IN ('vigencia', 'antiguedad'))
            AND NOT EXISTS (SELECT 1 FROM trip_requirements r
                             WHERE r.trip_id = t.id AND r.key NOT IN ('vigencia', 'antiguedad')
                               AND r.status <> 'cumple'))                 AS requisitos,
           (coalesce(btrim(cu.email), '') <> ''
            AND coalesce(btrim(cu.phone_number), '') <> '')               AS contacto,
           pa.total_usd IS NOT NULL                                       AS total_usd,
           pa.total_ars IS NOT NULL                                       AS total_ars,
           pa.rate IS NOT NULL                                            AS cambio,
           coalesce(btrim(pa.method), '') <> ''                           AS metodo
      FROM trips t
      JOIN customers cu ON cu.id = t.customer_id
      LEFT JOIN payments pa ON pa.trip_id = t.id
     WHERE t.id = $1
),
evaluado AS (
    SELECT c.id,
           c.closing,
           CASE c.derives
             WHEN 'viaje.destino'                THEN d.destino
             WHEN 'viaje.salida'                 THEN d.salida
             WHEN 'viaje.regreso'                THEN d.regreso
             WHEN 'pasajero.nombre'              THEN d.nombre
             WHEN 'pasajero.apellido'            THEN d.apellido
             WHEN 'pasajero.titular'             THEN d.titular
             WHEN 'pasajero.documentos_vigentes' THEN d.documentos
             WHEN 'pasajero.requisitos'          THEN d.requisitos
             WHEN 'cliente.mail_y_telefono'      THEN d.contacto
             WHEN 'pago.total_usd'               THEN d.total_usd
             WHEN 'pago.total_ars'               THEN d.total_ars
             WHEN 'pago.cambio'                  THEN d.cambio
             WHEN 'pago.metodo'                  THEN d.metodo
             ELSE false
           END AS cumplido,
           CASE WHEN c.derives = 'pasajero.requisitos' THEN coalesce((
                  -- El aviso de requisitos prueba lo que avisó: si aparece un
                  -- requisito pendiente que no estaba, ya no lo cubre. Las
                  -- evidencias anteriores a la migración 0006 no guardaron qué
                  -- avisaron y siguen valiendo como antes.
                  SELECT e.notified IS NULL OR NOT EXISTS (
                           SELECT 1 FROM trip_requirements r
                            WHERE r.trip_id = c.trip_id AND r.status <> 'cumple'
                              AND r.key NOT IN ('vigencia', 'antiguedad')
                              AND NOT e.notified @> jsonb_build_array(` + requirementID + `))
                    FROM control_evidence e WHERE e.control_id = c.id
                   ORDER BY e.closed_at DESC LIMIT 1), false)
                ELSE EXISTS (SELECT 1 FROM control_evidence e WHERE e.control_id = c.id)
           END AS con_prueba,
           c.status AS actual
      FROM controls c
      JOIN datos d ON d.trip_id = c.trip_id
     WHERE c.derives IS NOT NULL
),
destino AS (
    SELECT id, actual,
           CASE
             WHEN cumplido             THEN 'cerrado'
             WHEN closing = 'derivado' THEN 'abierto'
             WHEN con_prueba           THEN actual
             ELSE 'abierto'
           END AS nuevo
      FROM evaluado
)
UPDATE controls c
   SET status = d.nuevo
  FROM destino d
 WHERE c.id = d.id
   AND d.actual IS DISTINCT FROM d.nuevo
`
