package httpapi

import (
	"bytes"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/juanseaven/viajes-crm/server/internal/kernel/blobs"
	"github.com/juanseaven/viajes-crm/server/internal/kernel/httpx"
)

// inlineTypes son los únicos tipos que se sirven para que el navegador los
// muestre. El resto se fuerza a descarga.
//
// La lista es corta a propósito y se decide por EXTENSIÓN, no por el tipo que
// declaró el remitente. Todo lo que hay acá es contenido pasivo: se pinta o se
// reproduce, pero no ejecuta nada.
//
// Ausencias deliberadas:
//
//   - SVG. Es una imagen para casi todo el mundo y un documento con <script>
//     para el navegador. Es el vector clásico de XSS por subida de archivos.
//   - PDF. El visor integrado ejecuta JavaScript embebido. El panel los muestra
//     como enlace de descarga igual, así que no se pierde nada.
var inlineTypes = map[string]string{
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".gif":  "image/gif",
	".webp": "image/webp",
	".mp4":  "video/mp4",
	".webm": "video/webm",
	".ogg":  "audio/ogg",
	".oga":  "audio/ogg",
	".mp3":  "audio/mpeg",
	".m4a":  "audio/mp4",
	".wav":  "audio/wav",
}

// filesHandler sirve los archivos del almacenamiento local.
//
// Se monta SIN middleware de sesión, y es deliberado: la autorización la lleva la
// firma de la propia URL, igual que en una URL prefirmada de R2. Exigir además el
// JWT rompería el caso que tiene que funcionar — Evolution descargando un adjunto
// que estamos enviando, sin ser un usuario del panel.
//
// Solo existe cuando el almacenamiento es local. Con R2, los archivos los sirve
// Cloudflare y esta ruta no se monta.
//
// # Por qué el contenido se trata como hostil
//
// Acá vive lo que MANDA UN TERCERO: cualquiera que le escriba a una línea
// conectada elige los bytes y el nombre del archivo, y de ese nombre sale la
// extensión de la clave. Si se dejara que el navegador decidiera qué hacer con
// un `.html` o un `.svg`, abrir ese "documento" desde el panel ejecutaría código
// del remitente en el origen del backend.
func filesHandler(local *blobs.Local) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.URL.Path, "/files/")
		q := r.URL.Query()

		data, err := local.Open(key, q.Get("expires"), q.Get("sig"))
		if err != nil {
			// 404 para todo, también para la firma inválida: distinguirlo de "no
			// existe" le diría a quien pruebe enlaces al azar qué claves son
			// reales.
			httpx.NotFound(w, "no disponible")
			return
		}

		// Aunque el tipo se fije más abajo, nosniff evita que un navegador viejo
		// reinterprete el contenido por su cuenta.
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// Segunda barrera, por si algún tipo de la lista blanca resultara
		// ejecutable en algún navegador: sin orígenes permitidos y en sandbox, no
		// hay scripts, ni formularios, ni navegación.
		w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
		w.Header().Set("Cache-Control", "private, max-age=300")

		if mimeType, ok := inlineTypes[strings.ToLower(path.Ext(key))]; ok {
			w.Header().Set("Content-Type", mimeType)
			w.Header().Set("Content-Disposition", "inline")
		} else {
			// Desconocido o potencialmente activo: se descarga, nunca se muestra.
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Disposition",
				`attachment; filename="`+safeFileName(key)+`"`)
		}

		// ServeContent respeta el Content-Type ya fijado en vez de deducirlo de la
		// extensión, que es justamente lo que no queremos que decida. Se usa igual
		// porque da soporte de Range, y sin eso no se puede adelantar un audio.
		http.ServeContent(w, r, key, time.Time{}, bytes.NewReader(data))
	}
}

// safeFileName deja un nombre apto para la cabecera Content-Disposition.
//
// La clave llega de la URL, así que sin limpiarla se podrían inyectar cabeceras
// con un salto de línea, o romper el valor con una comilla.
func safeFileName(key string) string {
	name := path.Base(key)
	name = strings.Map(func(r rune) rune {
		switch {
		case r < 32, r == 127: // control, incluidos \r y \n
			return -1
		case r == '"', r == '\\':
			return -1
		case r > 126: // no ASCII: la cabecera solo admite latin-1 sin codificar
			return -1
		default:
			return r
		}
	}, name)

	if name == "" || name == "." || name == ".." {
		return "archivo"
	}
	if len(name) > 100 {
		name = name[:100]
	}
	return name
}
