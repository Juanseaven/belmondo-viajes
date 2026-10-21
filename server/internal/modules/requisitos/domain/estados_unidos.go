package domain

// Estados Unidos pide que el pasaporte venza al menos 6 meses después de la
// estadía, salvo a los ciudadanos de los países de esta lista, que solo
// necesitan el pasaporte vigente durante la estadía. Ninguna fuente
// automática lo dice bien: Wikipedia llega por descarte y las búsquedas no
// leen la lista, que es un PDF. Por eso es una constante, como Schengen.
//
// Publicación 5303-1225 del Carrier Liaison Program de la CBP, 18/12/2025.
// «Antilles» son las antiguas Antillas Neerlandesas: Curazao, Sint Maarten y
// el Caribe neerlandés.
var sixMonthClub = set(
	"AL", "AD", "AO", "AG", "CW", "SX", "BQ", "AR", "AM", "AW", "AU", "AT", "BS", "BB", "BE", "BZ", "BM", "BO",
	"BA", "BR", "BG", "MM", "CA", "CL", "CO", "CR", "CI", "HR", "CY", "CZ", "DK", "DM", "DO", "EG", "SV", "EE",
	"ET", "FM", "FJ", "FI", "FR", "GA", "GE", "DE", "GR", "GD", "GT", "GN", "GY", "HT", "HK", "HU", "IS", "IN",
	"ID", "IE", "IL", "IT", "JM", "JP", "XK", "LV", "LB", "LY", "LI", "LT", "LU", "MO", "MG", "MV", "MY", "MT",
	"MR", "MU", "MX", "MC", "MN", "ME", "MZ", "NP", "NL", "NZ", "NI", "NG", "MK", "NO", "PK", "PW", "PA", "PG",
	"PY", "PE", "PH", "PL", "PT", "QA", "RO", "RU", "SM", "SA", "RS", "SC", "SG", "SK", "SI", "ZA", "KR", "ES",
	"LK", "KN", "LC", "VC", "SR", "SE", "CH", "TW", "TH", "TT", "TN", "TR", "TV", "UA", "AE", "GB", "UY", "UZ",
	"VA", "VE", "ZW",
)

var sixMonthCite = Citation{Source: SourceConstante,
	Quote:    "Visitors traveling to the U.S. are required to be in possession of passports that are valid for six months beyond the period of their intended stay in the U.S. Citizens of the countries listed below are exempt from the six-month rule and need only have a passport valid for their intended period of stay.",
	URL:      "https://www.cbp.gov/sites/default/files/2025-12/Six-Month%20Passport%20Validity%20Update%2020251218.pdf",
	Revision: "2025-12-18"}

// usVigencia es la vigencia que pide Estados Unidos a una nacionalidad, con la
// explicación de por qué.
func usVigencia(nationality string) Field[Vigencia] {
	name := nationality
	if c, ok := CountryByCode(nationality); ok {
		name = c.Name
	}
	if sixMonthClub[nationality] {
		f := known(Vigencia{Amount: 0, Unit: "dias", Base: "salida_del_pais"}, sixMonthCite)
		f.Reason = "En general, Estados Unidos pide que el pasaporte venza al menos 6 meses después de la estadía. " +
			name + " está en la lista de países exceptuados (el «Six-Month Club»): alcanza con que esté vigente toda la estadía"
		return f
	}
	f := known(Vigencia{Amount: 6, Unit: "meses", Base: "salida_del_pais"}, sixMonthCite)
	f.Reason = "Estados Unidos pide que el pasaporte venza al menos 6 meses después de la estadía, y " + name +
		" no está en la lista de países exceptuados"
	return f
}
