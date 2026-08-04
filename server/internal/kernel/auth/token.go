package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var ErrInvalidToken = errors.New("token inválido")

// Claims son los datos que viajan firmados. Se mantiene mínimo a propósito: todo
// lo que se meta aquí queda congelado durante la vida del token, así que el rol
// y el estado del usuario se releen de la base en cada petición (ver
// Service.Authenticate). El id es lo único que no puede cambiar.
type Claims struct {
	UserID string `json:"sub"`
	jwt.RegisteredClaims
}

type Tokens struct {
	secret []byte
	ttl    time.Duration
}

func NewTokens(secret string, ttl time.Duration) (*Tokens, error) {
	// 32 bytes es lo que produce `openssl rand -hex 32` a medias; por debajo de
	// eso HS256 pierde el margen para el que está diseñado.
	if len(secret) < 32 {
		return nil, fmt.Errorf("JWT_SECRET debe tener al menos 32 caracteres")
	}
	if ttl <= 0 {
		ttl = 12 * time.Hour
	}
	return &Tokens{secret: []byte(secret), ttl: ttl}, nil
}

func (t *Tokens) TTL() time.Duration { return t.ttl }

func (t *Tokens) Issue(user User) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID: user.ID,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(t.ttl)),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(t.secret)
	if err != nil {
		return "", fmt.Errorf("firmando el token: %w", err)
	}
	return signed, nil
}

// Verify comprueba firma y vigencia.
//
// El algoritmo se fija explícitamente con WithValidMethods: sin eso, un atacante
// puede presentar un token con alg=none o alg=RS256 y la librería lo validaría
// contra un esquema que no elegimos. Es el fallo clásico de JWT.
func (t *Tokens) Verify(raw string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(raw, &Claims{},
		func(*jwt.Token) (any, error) { return t.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
	)
	if err != nil || !token.Valid {
		return nil, ErrInvalidToken
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || claims.UserID == "" {
		return nil, ErrInvalidToken
	}
	return claims, nil
}
