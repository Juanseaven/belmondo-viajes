package evaluacion

import (
	"fmt"
	"strings"
)

// Notice arma el mensaje de «Requisitos migratorios» para el cliente: lo que
// falta, por pasajero y en palabras.
//
// Lo que está en verificar no va: es tarea de la agencia. Y el mensaje dice
// «está todo en orden» solo si todo está en cumple; con algo pendiente de
// verificar o de cargar, dice que la agencia le confirma en breve.
//
// La vigencia y la antigüedad del documento son la excepción: las gobierna el
// control del vencimiento del documento, que donde la regla no sabe la
// vigencia exige que el documento llegue al regreso.
func Notice(holder string, t Trip, reqs []Requirement) string {
	byPassenger := map[string][]string{}
	doubts := false
	for _, r := range reqs {
		switch {
		case r.MissingData, r.Status == Verificar && !DocumentControl(r.Key):
			doubts = true
		case r.Status == Falta && r.ClientText != "":
			byPassenger[r.PassengerID] = append(byPassenger[r.PassengerID], r.ClientText)
		}
	}

	destinos := destinations(t)
	if len(byPassenger) == 0 {
		if doubts {
			return fmt.Sprintf("%s, estamos revisando los requisitos migratorios del viaje%s. Te confirmo en breve.", holder, destinos)
		}
		return fmt.Sprintf("%s, te confirmo que para el viaje%s está todo en orden con la documentación.", holder, destinos)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s, te paso lo que hace falta para el viaje%s:", holder, destinos)
	for _, p := range t.Passengers {
		if items := byPassenger[p.ID]; len(items) > 0 {
			fmt.Fprintf(&b, "\n• %s: %s.", p.Name, strings.Join(items, "; "))
		}
	}
	if doubts {
		b.WriteString("\nEl resto lo estamos revisando y te confirmo en breve.")
	}
	b.WriteString("\nCualquier duda, avisame.")
	return b.String()
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
