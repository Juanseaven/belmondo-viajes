package postgres

import "testing"

func TestPhoneFromJIDDoesNotTreatAGroupAsAPhoneNumber(t *testing.T) {
	t.Parallel()

	if got := phoneFromJID("120363321554617552@g.us"); got != "" {
		t.Fatalf("group phone = %q, want empty", got)
	}
	if got := phoneFromJID("5491126437432@s.whatsapp.net"); got != "+5491126437432" {
		t.Fatalf("contact phone = %q", got)
	}
}
