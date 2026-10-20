package evaluacion

import (
	reglas "github.com/juanseaven/viajes-crm/server/internal/modules/requisitos/domain"
)

// candidates son los documentos entre los que se elige: el elegido a mano, si
// alguien lo eligió; si no, todos.
func candidates(p Passenger) []Document {
	if p.DocManual || len(p.Docs) == 0 {
		if p.Doc == nil {
			return nil
		}
		return []Document{*p.Doc}
	}
	return p.Docs
}

// ChooseDocuments elige con qué documento viaja cada pasajero que no lo eligió
// a mano: el que deja menos «falta» en todo el itinerario y, después, menos
// «verificar». Si empatan, el DNI (a Brasil se viaja con DNI), después el que ya
// tenía y después el que vence más tarde. Devuelve una copia del viaje.
func ChooseDocuments(t Trip, rules map[reglas.Pair]reglas.Rule) Trip {
	out := t
	out.Passengers = make([]Passenger, len(t.Passengers))
	for i, p := range t.Passengers {
		out.Passengers[i] = p
		docs := candidates(p)
		if p.DocManual || len(docs) < 2 {
			if !p.DocManual && len(docs) == 1 {
				out.Passengers[i].Doc = &docs[0]
			}
			continue
		}
		best, bestScore := 0, score{}
		for j := range docs {
			sc := scoreFor(t, p, docs[j], rules)
			if j == 0 || sc.better(bestScore) {
				best, bestScore = j, sc
			}
		}
		out.Passengers[i].Doc = &docs[best]
	}
	return out
}

type score struct {
	falta, verificar int
	dni, current     bool
	expires          int64
}

func (a score) better(b score) bool {
	switch {
	case a.falta != b.falta:
		return a.falta < b.falta
	case a.verificar != b.verificar:
		return a.verificar < b.verificar
	case a.dni != b.dni:
		return a.dni
	case a.current != b.current:
		return a.current
	}
	return a.expires > b.expires
}

func scoreFor(t Trip, p Passenger, doc Document, rules map[reglas.Pair]reglas.Rule) score {
	cand := p
	cand.Doc, cand.DocManual = &doc, true
	trip := t
	trip.Passengers = []Passenger{cand}
	sc := score{dni: doc.Type == "DNI", current: p.Doc != nil && p.Doc.ID != "" && p.Doc.ID == doc.ID}
	if doc.ExpiresAt != nil {
		sc.expires = doc.ExpiresAt.Unix()
	}
	for _, r := range Evaluate(trip, rules, nil) {
		switch r.Status {
		case Falta:
			sc.falta++
		case Verificar:
			sc.verificar++
		}
	}
	return sc
}
