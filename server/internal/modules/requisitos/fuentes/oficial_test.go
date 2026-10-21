package fuentes

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/juanseaven/viajes-crm/server/internal/modules/requisitos/domain"
)

// fakeAnthropicSeq responde, en orden, una respuesta por pedido y guarda cada
// pedido.
func fakeAnthropicSeq(t *testing.T, responses ...string) (*httptest.Server, *[]map[string]any) {
	t.Helper()
	var mu sync.Mutex
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		raw, _ := io.ReadAll(r.Body)
		body := map[string]any{}
		_ = json.Unmarshal(raw, &body)
		bodies = append(bodies, body)
		w.Header().Set("Content-Type", "application/json")
		if len(bodies) > len(responses) {
			http.Error(w, `{"type":"error","error":{"type":"api_error","message":"de más"}}`, http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(responses[len(bodies)-1]))
	}))
	t.Cleanup(srv.Close)
	return srv, &bodies
}

const paginaOficial = `<html><body><h1>Visitor visa</h1>
<p>Argentine citizens need a visitor visa (B-2) for tourism.</p>
<p>Complete the online application, Form DS-160, and schedule an interview at the U.S. Embassy in Buenos Aires.</p>
</body></html>`

func busquedaConPausa(pageURL, blockedURL string) string {
	return `{"id":"msg_1","type":"message","role":"assistant","model":"claude-opus-5-5","stop_reason":"pause_turn",
	"content":[
	  {"type":"server_tool_use","id":"srvtoolu_1","name":"web_search","input":{"query":"US visitor visa Argentina"}},
	  {"type":"web_search_tool_result","tool_use_id":"srvtoolu_1","content":[
	    {"type":"web_search_result","url":"` + pageURL + `","title":"Visitor visa","encrypted_content":"x","page_age":null}]},
	  {"type":"server_tool_use","id":"srvtoolu_2","name":"web_fetch","input":{"url":"` + blockedURL + `?os=dio"}},
	  {"type":"web_fetch_tool_result","tool_use_id":"srvtoolu_2","content":{"type":"web_fetch_result","url":"` + blockedURL + `?os=dio",
	    "retrieved_at":"2026-10-21T07:42:18+00:00","content":{"type":"document","title":"Visitor Visa",
	    "source":{"type":"text","media_type":"text/plain","data":"# Visitor Visa\n\nInterviews are **generally required** for visa applicants."}}}}
	],
	"usage":{"input_tokens":5000,"output_tokens":200,"server_tool_use":{"web_search_requests":2,"web_fetch_requests":0}}}`
}

func notas(text string) string {
	raw, _ := json.Marshal(text)
	return `{"id":"msg_2","type":"message","role":"assistant","model":"claude-opus-5-5","stop_reason":"end_turn",
	"content":[{"type":"text","text":` + string(raw) + `}],
	"usage":{"input_tokens":3000,"output_tokens":400,"cache_read_input_tokens":20000,"cache_creation_input_tokens":4000,
	  "server_tool_use":{"web_search_requests":1,"web_fetch_requests":1}}}`
}

func TestLaBusquedaOficialDevuelveSoloLoComprobado(t *testing.T) {
	pages := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Como travel.state.gov: a un robot cualquiera no le responde.
		if r.URL.Path == "/bloqueada" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(paginaOficial))
	}))
	t.Cleanup(pages.Close)
	pageURL := pages.URL + "/visitor"

	composed, _ := json.Marshal(domain.ResearchOutput{
		Grupos: []domain.GrupoBuscado{{Grupo: domain.GrupoVisa, Tramite: []domain.FraseBuscada{
			{Text: "Se completa el formulario DS-160 online y hay entrevista en la embajada en Buenos Aires.",
				URL: pageURL, Quote: "Complete the online application, Form DS-160, and schedule an interview at the U.S. Embassy in Buenos Aires."},
			// El servidor no la puede bajar, pero la herramienta de lectura sí:
			// vale su texto. La herramienta la leyó con «?os=dio» y la cita no
			// lo lleva: es la misma página.
			{Text: "En general hay entrevista.", URL: pageURL[:strings.LastIndex(pageURL, "/")] + "/bloqueada",
				Quote: "Interviews are generally required for visa applicants."},
			// Esta página existe y dice eso, pero no apareció en la búsqueda.
			{Text: "Hay que pedir la visa de visitante para turismo.", URL: pages.URL + "/no-leida",
				Quote: "Argentine citizens need a visitor visa"},
		}}},
	})
	srv, bodies := fakeAnthropicSeq(t,
		busquedaConPausa(pageURL, pages.URL+"/bloqueada"),
		notas("[visa] Se completa el DS-160 | "+pageURL+" | Complete the online application, Form DS-160"),
		message(string(composed), "end_turn"),
	)
	o := NewOficial("clave-de-test", srv.URL, "claude-opus-5-5", "agencia@example.com")
	o.now = func() time.Time { return time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC) }
	// La página de prueba vive en 127.0.0.1, que el cliente de verdad rechaza.
	o.http = pages.Client()
	base := domain.Fields{Visa: domain.Field[domain.Visa]{Value: ptr(domain.VisaConsular)}}

	res, usage, err := o.Research(context.Background(), domain.Pair{Nationality: "AR", Country: "US"}, "Estados Unidos", base)
	if err != nil {
		t.Fatalf("búsqueda: %v", err)
	}
	g := res.Grupos[domain.GrupoVisa]
	if len(g.Tramite) != 2 || g.Tramite[0].Source.URL != pageURL || g.Tramite[0].Source.ReadAt != "2026-10-21" ||
		g.Tramite[1].Source.URL != pages.URL+"/bloqueada" {
		t.Fatalf("trámite = %+v", res.Grupos)
	}
	if usage.WebSearches != 3 || usage.InputTokens != 5000+3000+1200 || usage.Model != "claude-opus-5-5" ||
		usage.CacheReadTokens != 20000 || usage.CacheWriteTokens != 4000 {
		t.Errorf("consumo = %+v", usage)
	}

	if len(*bodies) != 3 {
		t.Fatalf("pedidos = %d: búsqueda, continuación de la pausa y armado", len(*bodies))
	}
	tools, _ := json.Marshal((*bodies)[0]["tools"])
	for _, want := range []string{"web_search_20260209", "web_fetch_20260209", "ivisa.com", "wikipedia.org",
		`"max_content_tokens":12000`, `"max_uses":4`, `"max_uses":8`} {
		if !strings.Contains(string(tools), want) {
			t.Errorf("las herramientas no tienen %q: %s", want, tools)
		}
	}
	// Lo que se vuelve a leer en cada vuelta sale de la caché.
	for i, body := range *bodies {
		if body["cache_control"] == nil {
			t.Errorf("el pedido %d no usa la caché", i)
		}
	}
	// La pausa se continúa con lo que el modelo ya hizo.
	cont, _ := json.Marshal((*bodies)[1]["messages"])
	if !strings.Contains(string(cont), "srvtoolu_1") {
		t.Errorf("la continuación no lleva el turno anterior: %s", cont)
	}
	// El armado lee las notas y la regla.
	compose, _ := json.Marshal((*bodies)[2])
	if !strings.Contains(string(compose), "DS-160") || !strings.Contains(string(compose), "json_schema") {
		t.Errorf("el armado no lleva las notas o el esquema: %s", compose)
	}
}

func ptr[T any](v T) *T { return &v }

// Las URLs a bajar salen de resultados de búsqueda que vienen de la web: el
// servidor no puede terminar pidiendo una dirección interna, ni por HTTP plano,
// ni después de una redirección.
func TestLaComprobacionNoBajaDireccionesInternas(t *testing.T) {
	interna := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("secreto"))
	}))
	t.Cleanup(interna.Close)
	o := NewOficial("clave-de-test", "", "", "")

	for _, u := range []string{interna.URL + "/", "http://example.com/", "file:///etc/passwd"} {
		if _, err := o.page(context.Background(), u); err == nil {
			t.Errorf("%s se bajó", u)
		}
	}

	for _, ip := range []string{"127.0.0.1", "10.0.0.5", "172.18.0.3", "192.168.1.1", "169.254.169.254", "100.64.0.1", "::1", "fd00::1", "fe80::1", "0.0.0.0"} {
		if publicIP(net.ParseIP(ip)) {
			t.Errorf("%s cuenta como pública", ip)
		}
	}
	if !publicIP(net.ParseIP("8.8.8.8")) || !publicIP(net.ParseIP("2001:4860:4860::8888")) {
		t.Error("una dirección pública se rechaza")
	}
}

// Haiku no tiene respaldo del lado del servidor ante un rechazo: mandárselo
// haría fallar el pedido.
func TestConHaikuNoSePideRespaldo(t *testing.T) {
	srv, bodies := fakeAnthropicSeq(t, notas("sin notas"), message(`{"grupos":[],"control":{"visa":null,"documento":null,"vigencia":null,"estadiaMaxDias":null}}`, "end_turn"))
	o := NewOficial("clave-de-test", srv.URL, "claude-haiku-5-5", "")
	if _, _, err := o.Research(context.Background(), domain.Pair{Nationality: "AR", Country: "US"}, "Estados Unidos", domain.Fields{}); err != nil {
		t.Fatal(err)
	}
	for i, body := range *bodies {
		if body["model"] != "claude-haiku-5-5" || body["fallbacks"] != nil {
			t.Errorf("pedido %d: modelo %v, respaldo %v", i, body["model"], body["fallbacks"])
		}
	}
}
