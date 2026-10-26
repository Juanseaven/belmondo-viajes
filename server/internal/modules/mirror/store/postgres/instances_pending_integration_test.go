package postgres

import "testing"

// Solo se retoma una línea que nunca llegó a vincularse. Una que ya estuvo
// conectada tiene historial propio y volver a vincularla es otro flujo.
func TestPendingByLabelOnlyReturnsLinesThatNeverConnected(t *testing.T) {
	ctx, p := testDB(t)
	instances := NewInstances(p)

	pendingID, err := instances.Create(ctx, "ventas-pendiente", "Ventas", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `UPDATE instances SET status='connecting' WHERE id=$1`, pendingID); err != nil {
		t.Fatal(err)
	}
	linkedID, err := instances.Create(ctx, "soporte-vinculada", "Soporte", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx,
		`UPDATE instances SET status='close', connected_at=now(), phone_jid='5491100000000@s.whatsapp.net' WHERE id=$1`,
		linkedID); err != nil {
		t.Fatal(err)
	}

	line, ok, err := instances.PendingByLabel(ctx, "Ventas")
	if err != nil || !ok || line.ID != pendingID || line.Name != "ventas-pendiente" {
		t.Fatalf("PendingByLabel(Ventas) = %+v ok=%v err=%v", line, ok, err)
	}
	if _, ok, err := instances.PendingByLabel(ctx, "Soporte"); err != nil || ok {
		t.Fatalf("PendingByLabel(Soporte) ok=%v err=%v, want no pending line", ok, err)
	}
	if _, ok, err := instances.PendingByLabel(ctx, "Inexistente"); err != nil || ok {
		t.Fatalf("PendingByLabel(Inexistente) ok=%v err=%v", ok, err)
	}
}
