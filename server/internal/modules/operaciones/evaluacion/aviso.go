package evaluacion

import (
	"fmt"
	"strings"
)

// Notice arma el mensaje de «Requisitos migratorios» para el cliente: lo que
// falta, por pasajero y en palabras, con cómo se tramita cuando se sabe.
//
// Lo que está en verificar no va: es tarea de la agencia. Y el mensaje dice
// «está todo en orden» solo si todo está en cumple; con algo pendiente de
// verificar o de cargar, dice que la agencia le confirma en breve. Las
// recomendaciones (renovar el pasaporte) van al final, aunque esté todo en
// orden: no son faltantes.
//
// La vigencia y la antigüedad del documento son la excepción: las gobierna el
// control del vencimiento del documento, que donde la regla no sabe la
// vigencia exige que el documento llegue al regreso.
func Notice(holder string, t Trip, reqs []Requirement) string {
	byPassenger := map[string][]string{}
	var advice []string
	doubts := false
	for _, r := range reqs {
		switch {
		case r.Status == Aviso:
			if r.ClientText != "" {
				advice = append(advice, r.ClientText)
			}
		case r.MissingData, r.Status == Verificar && !DocumentControl(r.Key):
			doubts = true
		case r.Status == Falta && r.ClientText != "":
			byPassenger[r.PassengerID] = append(byPassenger[r.PassengerID], r.ClientText)
		}
	}

	var b strings.Builder
	destinos := destinations(t)
	switch {
	case len(byPassenger) > 0:
		fmt.Fprintf(&b, "%s, te paso lo que hace falta para el viaje%s:", holder, destinos)
		for _, p := range t.Passengers {
			if items := byPassenger[p.ID]; len(items) > 0 {
				fmt.Fprintf(&b, "\n• %s: %s", p.Name, sentences(items))
			}
		}
		if doubts {
			b.WriteString("\nEl resto lo estamos revisando y te confirmo en breve.")
		}
	case doubts:
		fmt.Fprintf(&b, "%s, estamos revisando los requisitos migratorios del viaje%s. Te confirmo en breve.", holder, destinos)
	default:
		fmt.Fprintf(&b, "%s, te confirmo que para el viaje%s está todo en orden con la documentación.", holder, destinos)
	}
	for _, a := range advice {
		fmt.Fprintf(&b, "\nTe recomendamos %s.", strings.TrimSuffix(a, "."))
	}
	if len(byPassenger) > 0 {
		b.WriteString("\nCualquier duda, avisame.")
	}
	return b.String()
}

// sentences arma frases completas con los ítems de un pasajero: cada uno
// empieza en mayúscula y termina en punto.
func sentences(items []string) string {
	out := make([]string, len(items))
	for i, it := range items {
		it = capitalize(strings.TrimSpace(it))
		if !strings.HasSuffix(it, ".") {
			it += "."
		}
		out[i] = it
	}
	return strings.Join(out, " ")
}

// DocumentControl dice si el requisito lo gobierna el control del vencimiento
// del documento y no el de requisitos migratorios.
func DocumentControl(key string) bool { return key == KeyVigencia || key == KeyAntiguedad }

func destinations(t Trip) string {
	var names []string
	for _, s := range t.Itinerary {
		if s.Role == "destino" {
			names = append(names, countryName(s.Country))
		}
	}
	switch len(names) {
	case 0:
		return ""
	case 1:
		return " a " + names[0]
	default:
		return " a " + strings.Join(names[:len(names)-1], ", ") + " y " + names[len(names)-1]
	}
}
