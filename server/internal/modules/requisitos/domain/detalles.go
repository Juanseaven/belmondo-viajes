package domain

import "strconv"

// Frase es una oración del detalle que sale de las páginas oficiales, con la
// página y la cita textual que la sostienen.
type Frase struct {
	Text   string   `json:"text"`
	Source Citation `json:"source"`
}

// Grupo es el detalle de un requisito: la regla explicada (la general y la
// excepción que le toca a esta nacionalidad) y, si hay que hacer algo, el
// trámite.
type Grupo struct {
	Pide    []Frase `json:"pide,omitempty"`
	Tramite []Frase `json:"tramite,omitempty"`
}

// Los grupos del detalle. Las condiciones usan la clave de su renglón
// (CondicionKeys).
const (
	GrupoDocumento  = "documento"
	GrupoVisa       = "visa"
	GrupoVigencia   = "vigencia"
	GrupoAntiguedad = "antiguedad"
	GrupoEstadia    = "estadia"
	GrupoVacunas    = "vacunas"
	GrupoTransito   = "transito"
)

// DetallesStatus dice en qué anda la búsqueda en páginas oficiales.
type DetallesStatus string

const (
	DetallesBuscando DetallesStatus = "buscando"
	DetallesListo    DetallesStatus = "listo"
	// DetallesSinDatos: la búsqueda no encontró nada que pasara la comprobación.
	DetallesSinDatos DetallesStatus = "sin_datos"
)

// Detalles es lo que la regla sabe de las páginas oficiales, por grupo. Solo
// trae los grupos que valen para los campos actuales de la regla: el de un
// campo que cambió después de la búsqueda se saca hasta la búsqueda siguiente.
type Detalles struct {
	Status DetallesStatus   `json:"status,omitempty"`
	Grupos map[string]Grupo `json:"grupos,omitempty"`
}

// CondicionKeys da una clave por condición, la del renglón que la evalúa: si
// se repite el tipo, numera («condicion:otra», «condicion:otra:2»).
func CondicionKeys(cs []Condicion) []string {
	seen := map[string]int{}
	keys := make([]string, len(cs))
	for i, c := range cs {
		seen[c.Kind]++
		keys[i] = CondicionPrefix + c.Kind
		if n := seen[c.Kind]; n > 1 {
			keys[i] += ":" + strconv.Itoa(n)
		}
	}
	return keys
}

// CondicionPrefix es el prefijo de las claves de las condiciones.
const CondicionPrefix = "condicion:"
