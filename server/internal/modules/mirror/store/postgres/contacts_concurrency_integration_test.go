package postgres

import (
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/juanseaven/viajes-crm/server/internal/modules/mirror/domain"
)

// La agenda se guarda desde varios lugares a la vez: la recuperación de la
// línea al arrancar y cada lote de historial. Cada fuente trae los contactos en
// su propio orden; si cada transacción los bloquea en ese orden, dos que se
// cruzan terminan en «deadlock detected».
func TestConcurrentDirectoryUpsertsDoNotDeadlock(t *testing.T) {
	ctx, p := testDB(t)
	seedLine(t, ctx, p, "agenda", "Agenda")

	contacts := make([]domain.ContactIdentity, 300)
	for i := range contacts {
		contacts[i] = domain.ContactIdentity{
			PhoneJID: fmt.Sprintf("54911%08d@s.whatsapp.net", i),
			FullName: fmt.Sprintf("Contacto %d", i),
		}
	}
	reversed := slices.Clone(contacts)
	slices.Reverse(reversed)

	directory := NewContacts(p)
	for round := 0; round < 5; round++ {
		var wg sync.WaitGroup
		errs := make(chan error, 2)
		for _, batch := range [][]domain.ContactIdentity{contacts, reversed} {
			wg.Add(1)
			go func(batch []domain.ContactIdentity) {
				defer wg.Done()
				errs <- directory.UpsertDirectory(ctx, "agenda", batch)
			}(batch)
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("ronda %d: %v", round, err)
			}
		}
	}
}
