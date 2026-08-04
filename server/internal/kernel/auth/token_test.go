package auth

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "0123456789abcdef0123456789abcdef"

func newTestTokens(t *testing.T, ttl time.Duration) *Tokens {
	t.Helper()
	tokens, err := NewTokens(testSecret, ttl)
	if err != nil {
		t.Fatalf("NewTokens: %v", err)
	}
	return tokens
}

func TestNewTokensRechazaSecretosCortos(t *testing.T) {
	if _, err := NewTokens("corto", time.Hour); err == nil {
		t.Fatal("se aceptó un JWT_SECRET de 5 caracteres")
	}
}

func TestIssueYVerifyDanLaVuelta(t *testing.T) {
	tokens := newTestTokens(t, time.Hour)

	raw, err := tokens.Issue(User{ID: "u-1", Role: RoleAdmin})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	claims, err := tokens.Verify(raw)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.UserID != "u-1" {
		t.Fatalf("UserID = %q, se esperaba u-1", claims.UserID)
	}
}

func TestVerifyRechaza(t *testing.T) {
	tokens := newTestTokens(t, time.Hour)
	valid, err := tokens.Issue(User{ID: "u-1"})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	otherSecret, err := NewTokens("ffffffffffffffffffffffffffffffff", time.Hour)
	if err != nil {
		t.Fatalf("NewTokens: %v", err)
	}
	firmadoPorOtro, err := otherSecret.Issue(User{ID: "u-1"})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	cases := []struct {
		name  string
		token string
	}{
		{"vacío", ""},
		{"basura", "no-es-un-jwt"},
		{"firmado con otro secreto", firmadoPorOtro},
		{"firma alterada", valid[:len(valid)-3] + "aaa"},
		{"payload alterado", alterarPayload(valid)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := tokens.Verify(c.token); !errors.Is(err, ErrInvalidToken) {
				t.Fatalf("Verify(%s) = %v, se esperaba ErrInvalidToken", c.name, err)
			}
		})
	}
}

func TestVerifyRechazaTokenExpirado(t *testing.T) {
	// TTL negativo: nace vencido, sin que el test tenga que esperar.
	tokens := newTestTokens(t, time.Hour)
	tokens.ttl = -time.Minute

	raw, err := tokens.Issue(User{ID: "u-1"})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if _, err := tokens.Verify(raw); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("Verify de un token expirado = %v, se esperaba ErrInvalidToken", err)
	}
}

// El fallo clásico de JWT: presentar un token con alg=none para que la librería
// se salte la verificación de firma. WithValidMethods lo corta.
func TestVerifyRechazaAlgNone(t *testing.T) {
	tokens := newTestTokens(t, time.Hour)

	unsigned, err := jwt.NewWithClaims(jwt.SigningMethodNone, Claims{
		UserID: "u-1",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("construyendo el token sin firma: %v", err)
	}

	if _, err := tokens.Verify(unsigned); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("se aceptó un token con alg=none: %v", err)
	}
}

// Un token bien firmado pero sin `sub` no identifica a nadie.
func TestVerifyRechazaTokenSinSujeto(t *testing.T) {
	tokens := newTestTokens(t, time.Hour)

	raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("firmando: %v", err)
	}

	if _, err := tokens.Verify(raw); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("se aceptó un token sin sub: %v", err)
	}
}

// alterarPayload cambia un carácter del cuerpo dejando la firma original, que es
// lo que haría alguien intentando escalar privilegios editando sus claims.
func alterarPayload(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return token
	}
	body := []byte(parts[1])
	if body[0] == 'e' {
		body[0] = 'f'
	} else {
		body[0] = 'e'
	}
	return parts[0] + "." + string(body) + "." + parts[2]
}
