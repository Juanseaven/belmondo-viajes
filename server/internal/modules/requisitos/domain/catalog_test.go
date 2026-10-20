package domain

import "testing"

func TestElCatalogoDaNombresEnEspanolYLaPaginaDeVisas(t *testing.T) {
	ar, ok := CountryByCode("AR")
	if !ok || ar.Name != "Argentina" || ar.VisaPage != "Visa requirements for Argentine citizens" {
		t.Fatalf("AR = %+v, %v", ar, ok)
	}
	us, _ := CountryByCode("US")
	if us.Name != "Estados Unidos" {
		t.Errorf("US = %q", us.Name)
	}
	if _, ok := CountryByCode("ZZ"); ok {
		t.Error("ZZ no es un país")
	}
}

func TestLosPaisesSeListanPorNombre(t *testing.T) {
	all := Countries()
	if len(all) < 190 {
		t.Fatalf("países = %d", len(all))
	}
	for i := 1; i < len(all); i++ {
		if collate(all[i-1].Name) > collate(all[i].Name) {
			t.Fatalf("%q está antes que %q", all[i-1].Name, all[i].Name)
		}
	}
}

func TestSchengenYLaUnion(t *testing.T) {
	if !InSchengen("ES") || InSchengen("GB") || InSchengen("AR") {
		t.Error("España es Schengen; Reino Unido y Argentina no")
	}
	if !FreeMovementInSchengen("IT") || !FreeMovementInSchengen("CH") || FreeMovementInSchengen("AR") {
		t.Error("Italia y Suiza circulan libres en Schengen; Argentina no")
	}
}
