package postgres

import (
	"errors"
	"testing"

	"github.com/juanseaven/viajes-crm/server/internal/modules/operaciones/store"
)

// Protege la mutación que más fácil deja mentir al checklist: si el store no
// recalcula dentro de la misma transacción, borrar el último pasajero deja los
// controles de nombre, apellido y titular cerrados.
func TestPasajerosCierranYReabrenLosDerivados(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	user := seedUser(t, ctx, pool, "pasajeros@agencia.test")
	contact := seedContact(t, ctx, pool, "5491100000101@s.whatsapp.net", "Ana Pérez")
	trip, err := s.CreateTrip(ctx, store.NewTrip{
		ContactID: contact, Destination: "Iguazú", DepartAt: fecha("2026-11-10"),
		ReturnAt: fecha("2026-11-17"), CreatedBy: user,
	})
	if err != nil {
		t.Fatalf("alta del viaje: %v", err)
	}

	passenger, err := s.AddPassenger(ctx, store.NewPassenger{
		TripID: trip.ID, FirstName: "Ana", LastName: "Pérez", Kind: "adulto",
		IsHolder: true, DocumentType: "DNI", DocumentNumber: "30111222", CreatedBy: user,
	})
	if err != nil {
		t.Fatalf("alta del pasajero: %v", err)
	}
	for _, id := range []string{"carga-nombres", "carga-apellidos", "control-titular"} {
		if got := estado(t, ctx, pool, trip.ID, id); got != "cerrado" {
			t.Errorf("%s quedó %s después del alta, se esperaba cerrado", id, got)
		}
	}

	if err := s.DeletePassenger(ctx, passenger.ID); err != nil {
		t.Fatalf("baja del pasajero: %v", err)
	}
	for _, id := range []string{"carga-nombres", "carga-apellidos", "control-titular"} {
		if got := estado(t, ctx, pool, trip.ID, id); got != "abierto" {
			t.Errorf("%s quedó %s después de borrar el dato, se esperaba abierto", id, got)
		}
	}
}

func TestBorrarPasajeroInexistenteDevuelveNotFound(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	err := s.DeletePassenger(ctx, "00000000-0000-0000-0000-000000000099")
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("error = %v, se esperaba ErrNotFound", err)
	}
}

// Protege la restricción de negocio observable: una segunda persona no puede
// reemplazar silenciosamente al titular ya registrado.
func TestPasajerosRechazanUnSegundoTitular(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	user := seedUser(t, ctx, pool, "titular@agencia.test")
	contact := seedContact(t, ctx, pool, "5491100000102@s.whatsapp.net", "Familia Rossi")
	trip, err := s.CreateTrip(ctx, store.NewTrip{
		ContactID: contact, Destination: "Salta", DepartAt: fecha("2026-11-10"),
		ReturnAt: fecha("2026-11-17"), CreatedBy: user,
	})
	if err != nil {
		t.Fatalf("alta del viaje: %v", err)
	}
	for _, name := range []string{"Lucía", "Martín"} {
		_, err = s.AddPassenger(ctx, store.NewPassenger{
			TripID: trip.ID, FirstName: name, LastName: "Rossi", Kind: "adulto",
			IsHolder: true, CreatedBy: user,
		})
		if name == "Lucía" && err != nil {
			t.Fatalf("primer titular: %v", err)
		}
	}
	if err != store.ErrConflict {
		t.Fatalf("segundo titular: error %v, se esperaba ErrConflict", err)
	}
}

func TestEditarPasajeroActualizaLosDatosYElTitularDerivado(t *testing.T) {
	ctx, pool := testDB(t)
	s := New(pool)
	user := seedUser(t, ctx, pool, "editar-pasajero@agencia.test")
	contact := seedContact(t, ctx, pool, "5491100000105@s.whatsapp.net", "Julia López")
	trip, err := s.CreateTrip(ctx, store.NewTrip{
		ContactID: contact, Destination: "Córdoba", DepartAt: fecha("2026-11-10"),
		ReturnAt: fecha("2026-11-17"), CreatedBy: user,
	})
	if err != nil {
		t.Fatalf("alta del viaje: %v", err)
	}
	passenger, err := s.AddPassenger(ctx, store.NewPassenger{
		TripID: trip.ID, FirstName: "Juli", LastName: "Lopez", Kind: "adulto", CreatedBy: user,
	})
	if err != nil {
		t.Fatalf("alta del pasajero: %v", err)
	}
	firstName, lastName, holder := "Julia", "López", true
	updated, err := s.UpdatePassenger(ctx, passenger.ID, store.PassengerPatch{
		FirstName: &firstName, LastName: &lastName, IsHolder: &holder,
	})
	if err != nil {
		t.Fatalf("edición del pasajero: %v", err)
	}
	if updated.FirstName != "Julia" || updated.LastName != "López" || !updated.IsHolder {
		t.Fatalf("pasajero editado: %#v", updated)
	}
	if got := estado(t, ctx, pool, trip.ID, "control-titular"); got != "cerrado" {
		t.Errorf("titular editado dejó el control %s, se esperaba cerrado", got)
	}
}
