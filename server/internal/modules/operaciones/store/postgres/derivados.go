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
// `viaje.salida` y `viaje.regreso` son siempre verdaderos porque sus columnas
// son NOT NULL: el alta ya exigió las fechas. Que un renglón de la planilla sea
// siempre verdadero no es un defecto del cálculo; es la prueba de que era un
// campo y no un control.
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
           CASE c.derives
             WHEN 'viaje.destino'           THEN d.destino
             WHEN 'viaje.salida'            THEN d.salida
             WHEN 'viaje.regreso'           THEN d.regreso
             WHEN 'pasajero.nombre'         THEN d.nombre
             WHEN 'pasajero.apellido'       THEN d.apellido
             WHEN 'pasajero.titular'        THEN d.titular
             WHEN 'cliente.mail_y_telefono' THEN d.contacto
             WHEN 'pago.total_usd'          THEN d.total_usd
             WHEN 'pago.total_ars'          THEN d.total_ars
             WHEN 'pago.cambio'             THEN d.cambio
             WHEN 'pago.metodo'             THEN d.metodo
             ELSE false
           END AS cumplido,
           c.status AS actual
      FROM controls c
      JOIN datos d ON d.trip_id = c.trip_id
     WHERE c.closing = 'derivado'
)
UPDATE controls c
   SET status = CASE WHEN e.cumplido THEN 'cerrado' ELSE 'abierto' END
  FROM evaluado e
 WHERE c.id = e.id
   AND e.actual IS DISTINCT FROM CASE WHEN e.cumplido THEN 'cerrado' ELSE 'abierto' END
`
