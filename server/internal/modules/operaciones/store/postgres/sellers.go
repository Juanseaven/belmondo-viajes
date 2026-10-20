package postgres

import (
	"context"
	"fmt"

	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/store"
)

// Sellers lee la tabla users del kernel. Solo los activos: un usuario dado de
// baja conserva los viajes que vendió, pero no se le asignan nuevos.
func (s *Store) Sellers(ctx context.Context) ([]store.Seller, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, full_name FROM users WHERE is_active ORDER BY full_name, id`)
	if err != nil {
		return nil, fmt.Errorf("listando vendedores: %w", err)
	}
	defer rows.Close()

	sellers := []store.Seller{}
	for rows.Next() {
		var seller store.Seller
		if err := rows.Scan(&seller.ID, &seller.Name); err != nil {
			return nil, fmt.Errorf("leyendo vendedor: %w", err)
		}
		sellers = append(sellers, seller)
	}
	return sellers, rows.Err()
}
