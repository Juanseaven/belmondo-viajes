package auth

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestHashPasswordAceptaLaClaveTemporalDeLaDemo(t *testing.T) {
	const password = "admin1234"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword(%q): %v", password, err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		t.Fatalf("el hash no valida la contraseña original: %v", err)
	}
}
