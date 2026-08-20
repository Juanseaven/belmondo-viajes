package blobs

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"context"
)

// Local guarda los archivos en el disco del servidor.
//
// Es la implementación de desarrollo: el mismo puerto que R2, sin necesitar una
// cuenta ni credenciales. Cambiar de una a otra es una línea en el arranque.
//
// Conserva la propiedad que hace segura a la versión de R2: las URLs son
// **firmadas y de vida corta**. El archivo no se sirve por una ruta adivinable,
// así que reenviar el enlace por error no expone nada de forma permanente. Por
// acá viajan documentos de identidad de los clientes.
type Local struct {
	dir     string
	baseURL string
	secret  []byte
}

// NewLocal prepara el directorio.
//
// baseURL es la raíz pública del backend. Importa que sea alcanzable desde DOS
// sitios distintos: el navegador que muestra el archivo, y Evolution, que lo
// descarga cuando enviamos un adjunto.
func NewLocal(dir, baseURL, secret string) (*Local, error) {
	if dir == "" {
		return nil, errors.New("falta el directorio de almacenamiento")
	}
	if len(secret) < 32 {
		return nil, errors.New("el secreto de firma debe tener al menos 32 caracteres")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creando %s: %w", dir, err)
	}
	return &Local{
		dir:     dir,
		baseURL: strings.TrimSuffix(baseURL, "/"),
		secret:  []byte(secret),
	}, nil
}

var _ Store = (*Local)(nil)

func (l *Local) Put(_ context.Context, key, _ string, data []byte) error {
	path, err := l.resolve(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creando el directorio de %s: %w", key, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("escribiendo %s: %w", key, err)
	}
	return nil
}

// DeletePrefix elimina el directorio que representa un prefijo de objetos. Solo
// acepta prefijos terminados en barra para que nunca se confunda una línea con
// otra cuyo nombre empiece igual.
func (l *Local) DeletePrefix(_ context.Context, prefix string) error {
	if prefix == "" || !strings.HasSuffix(prefix, "/") {
		return fmt.Errorf("prefijo de borrado inválido: %q", prefix)
	}
	path, err := l.resolve(strings.TrimSuffix(prefix, "/"))
	if err != nil {
		return err
	}
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("borrando %s: %w", prefix, err)
	}
	return nil
}

// SignedURL emite un enlace temporal hacia la ruta de archivos del kernel.
func (l *Local) SignedURL(_ context.Context, key string, ttl time.Duration) (string, error) {
	if _, err := l.resolve(key); err != nil {
		return "", err
	}
	expires := time.Now().Add(ttl).Unix()
	return fmt.Sprintf("%s/files/%s?expires=%d&sig=%s",
		l.baseURL, key, expires, l.sign(key, expires)), nil
}

// Open devuelve el archivo si la firma es válida y no caducó. Lo usa la ruta que
// sirve los archivos.
func (l *Local) Open(key, expires, sig string) ([]byte, error) {
	exp, err := strconv.ParseInt(expires, 10, 64)
	if err != nil {
		return nil, ErrNotAuthorized
	}
	if time.Now().Unix() > exp {
		return nil, ErrNotAuthorized
	}
	// Comparación en tiempo constante: comparar firmas con == filtra por dónde
	// difieren y permite construirlas byte a byte.
	if !hmac.Equal([]byte(sig), []byte(l.sign(key, exp))) {
		return nil, ErrNotAuthorized
	}

	path, err := l.resolve(key)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, ErrNotFound
	}
	return data, nil
}

// ErrNotAuthorized indica firma inválida o caducada.
var ErrNotAuthorized = errors.New("enlace inválido o caducado")

// ErrNotFound indica que la clave no existe.
var ErrNotFound = errors.New("archivo no encontrado")

func (l *Local) sign(key string, expires int64) string {
	mac := hmac.New(sha256.New, l.secret)
	mac.Write([]byte(key))
	mac.Write([]byte{0}) // separador: evita que ("ab",1) y ("a",21) firmen igual
	mac.Write([]byte(strconv.FormatInt(expires, 10)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// resolve convierte una clave en una ruta del disco, comprobando que no se
// escape del directorio.
//
// La clave viene de la URL, así que sin esta comprobación un `../../etc/passwd`
// serviría cualquier archivo de la máquina. Es el fallo clásico de path
// traversal.
func (l *Local) resolve(key string) (string, error) {
	if key == "" || strings.Contains(key, "..") || strings.HasPrefix(key, "/") {
		return "", fmt.Errorf("clave inválida: %q", key)
	}
	// Se decodifica antes de comprobar: si no, un `%2e%2e%2f` esquivaría el filtro
	// de arriba y volvería a ser `../` al tocar el disco.
	decoded, err := url.PathUnescape(key)
	if err != nil || strings.Contains(decoded, "..") {
		return "", fmt.Errorf("clave inválida: %q", key)
	}

	path := filepath.Join(l.dir, filepath.FromSlash(decoded))
	root, err := filepath.Abs(l.dir)
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(abs, root+string(os.PathSeparator)) {
		return "", fmt.Errorf("clave fuera del directorio: %q", key)
	}
	return abs, nil
}
