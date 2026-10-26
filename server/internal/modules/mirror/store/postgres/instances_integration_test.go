package postgres

import (
	"testing"
)

func TestDeleteLineRemovesDirectoryOnlyContact(t *testing.T) {
	ctx, p := testDB(t)
	line := seedLine(t, ctx, p, "ventas", "Ventas")
	contact := seedContact(t, ctx, p,
		"5491111111111@s.whatsapp.net", "Contacto", "+5491111111111")

	if _, err := p.Exec(ctx,
		`INSERT INTO contact_directory (instance_id, wa_jid, full_name)
		 VALUES ($1, '5491111111111@s.whatsapp.net', 'Contacto')`, line); err != nil {
		t.Fatalf("sembrando agenda de la línea: %v", err)
	}

	if err := NewInstances(p).Delete(ctx, line); err != nil {
		t.Fatalf("borrando línea: %v", err)
	}

	var remaining int
	if err := p.QueryRow(ctx, `SELECT count(*) FROM contacts WHERE id = $1`, contact).Scan(&remaining); err != nil {
		t.Fatalf("contando contacto: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("contactos exclusivos restantes = %d; se esperaba 0", remaining)
	}
}

func TestDeleteLinePreservesContactFromAnotherDirectory(t *testing.T) {
	ctx, p := testDB(t)
	deletedLine := seedLine(t, ctx, p, "ventas", "Ventas")
	remainingLine := seedLine(t, ctx, p, "soporte", "Soporte")
	contact := seedContact(t, ctx, p,
		"5491222222222@s.whatsapp.net", "Compartido", "+5491222222222")

	if _, err := p.Exec(ctx,
		`INSERT INTO contact_directory (instance_id, wa_jid, full_name) VALUES
		 ($1, '5491222222222@s.whatsapp.net', 'Compartido'),
		 ($2, '5491222222222@s.whatsapp.net', 'Compartido')`, deletedLine, remainingLine); err != nil {
		t.Fatalf("sembrando agendas: %v", err)
	}

	if err := NewInstances(p).Delete(ctx, deletedLine); err != nil {
		t.Fatalf("borrando línea: %v", err)
	}

	var remaining int
	if err := p.QueryRow(ctx, `SELECT count(*) FROM contacts WHERE id = $1`, contact).Scan(&remaining); err != nil {
		t.Fatalf("contando contacto compartido: %v", err)
	}
	if remaining != 1 {
		t.Fatalf("contactos compartidos restantes = %d; se esperaba 1", remaining)
	}
}
