package fuentes

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/juanseaven/viajes-crm/server/internal/modules/requisitos/domain"
)

// fakeAnthropic responde como la API de mensajes y guarda lo que recibió.
func fakeAnthropic(t *testing.T, response string) (*httptest.Server, *map[string]any, *http.Header) {
	t.Helper()
	body := map[string]any{}
	headers := http.Header{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		headers = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	t.Cleanup(srv.Close)
	return srv, &body, &headers
}

const extraccionCanada = `{"documento":{"value":null,"quote":""},"visa":{"value":"visa_consular","quote":"Visa required"},
"estadia":{"value":null,"quote":""},"vigencia":{"value":null,"quote":""},"emisionMaxAnios":{"value":null,"quote":""},
"exenciones":[{"permitKinds":["visa"],"permitCountry":"US","visa":"autorizacion_electronica","quote":"hold a valid United States non-immigrant visa"}],
"vacunas":[],"condiciones":[{"kind":"seguro","text":"Tener seguro","quote":"insurance"}]}`

func message(text, stop string) string {
	raw, _ := json.Marshal(text)
	return `{"id":"msg_1","type":"message","role":"assistant","model":"claude-opus-5-5","stop_reason":"` + stop + `",
	"content":[{"type":"text","text":` + string(raw) + `}],"usage":{"input_tokens":1200,"output_tokens":300}}`
}

func TestClaudeDevuelveLaExtraccionYElConsumo(t *testing.T) {
	srv, body, headers := fakeAnthropic(t, message(extraccionCanada, "end_turn"))
	c := NewClaude("clave-de-test", srv.URL)

	ex, usage, err := c.Extract(context.Background(), domain.Pair{Nationality: "AR", Country: "CA"}, "Canadá", "| fila |")
	if err != nil {
		t.Fatalf("extracción: %v", err)
	}
	if ex.Visa.Value == nil || *ex.Visa.Value != domain.VisaConsular || len(ex.Exenciones) != 1 ||
		ex.Exenciones[0].PermitCountry != "US" || len(ex.Condiciones) != 1 || ex.Condiciones[0].Kind != domain.CondicionSeguro {
		t.Fatalf("extracción = %+v", ex)
	}
	if usage.InputTokens != 1200 || usage.OutputTokens != 300 || usage.Model != "claude-opus-5-5" {
		t.Errorf("consumo = %+v", usage)
	}

	if (*body)["model"] != "claude-opus-5-5" {
		t.Errorf("modelo = %v", (*body)["model"])
	}
	if (*body)["fallbacks"] != "default" {
		t.Errorf("fallbacks = %v", (*body)["fallbacks"])
	}
	if !strings.Contains(headers.Get("anthropic-beta"), "server-side-fallback-2026-07-01") {
		t.Errorf("beta = %q", headers.Get("anthropic-beta"))
	}
	format, _ := (*body)["output_config"].(map[string]any)["format"].(map[string]any)
	if format["type"] != "json_schema" || format["schema"] == nil {
		t.Errorf("output_config = %v", (*body)["output_config"])
	}
	raw, _ := json.Marshal((*body)["messages"])
	if !strings.Contains(string(raw), "| fila |") || !strings.Contains(string(raw), "Canadá") {
		t.Errorf("el pedido no lleva el texto ni el país: %s", raw)
	}
}

func TestClaudeAvisaSiRechazaElPedido(t *testing.T) {
	srv, _, _ := fakeAnthropic(t, message("", "refusal"))
	c := NewClaude("clave-de-test", srv.URL)

	if _, _, err := c.Extract(context.Background(), domain.Pair{Nationality: "AR", Country: "CA"}, "Canadá", "x"); err == nil {
		t.Fatal("un rechazo no es una extracción")
	}
}

func TestClaudeSinClaveNoExtrae(t *testing.T) {
	if NewClaude("", "") != nil {
		t.Fatal("sin clave no hay extractor: las reglas nuevas quedan sin fuentes")
	}
}
