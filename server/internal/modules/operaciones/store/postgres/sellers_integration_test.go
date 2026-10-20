package postgres

import "testing"

// El selector de vendedor ofrece a quien puede atender hoy: un usuario
// desactivado conserva sus viajes viejos, pero no se le asignan nuevos.
func TestVendedoresSonLosUsuariosActivosPorNombre(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	for _, u := range []struct {
		email, name string
		active      bool
	}{
		{"zoe@agencia.test", "Zoe Paz", true},
		{"ana@agencia.test", "Ana Gil", true},
		{"baja@agencia.test", "Beto Baja", false},
	} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO users (email, password_hash, full_name, is_active)
			 VALUES ($1, 'x', $2, $3)`, u.email, u.name, u.active); err != nil {
			t.Fatalf("sembrando %s: %v", u.email, err)
		}
	}

	sellers, err := s.Sellers(ctx)
	if err != nil {
		t.Fatalf("vendedores: %v", err)
	}
	if len(sellers) != 2 || sellers[0].Name != "Ana Gil" || sellers[1].Name != "Zoe Paz" || sellers[0].ID == "" {
		t.Fatalf("vendedores = %#v", sellers)
	}
}
