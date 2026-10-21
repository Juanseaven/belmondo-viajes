package evolution

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestContactsReturnsTheNamesSavedInTheLineAddressBook(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/user/contacts" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if got, want := r.Header.Get("apikey"), InstanceToken("global-secret", "juanse-abc"); got != want {
			t.Fatalf("apikey = %q, want derived instance token", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{
				"Jid": "5491126437432@s.whatsapp.net", "Found": true,
				"FirstName": "", "FullName": "Beto", "PushName": "BETOAVEN", "BusinessName": "",
			}},
		})
	}))
	defer server.Close()

	contacts, err := New(server.URL, "global-secret", "").Contacts(context.Background(), "juanse-abc")
	if err != nil {
		t.Fatalf("Contacts: %v", err)
	}
	if len(contacts) != 1 {
		t.Fatalf("contacts = %d, want 1", len(contacts))
	}
	got := contacts[0]
	if got.PhoneJID != "5491126437432@s.whatsapp.net" || got.FullName != "Beto" || got.PushName != "BETOAVEN" {
		t.Fatalf("contact = %+v", got)
	}
}

func TestAvatarURLUsesTheInstanceTokenAndReadsEvolutionURL(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/user/avatar" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if got, want := r.Header.Get("apikey"), InstanceToken("global-secret", "juanse-abc"); got != want {
			t.Fatalf("apikey = %q, want %q", got, want)
		}
		var body struct {
			Number  string `json:"number"`
			Preview bool   `json:"preview"`
		}
		decodeRequest(t, r, &body)
		if body.Number != "5491126437432@s.whatsapp.net" || !body.Preview {
			t.Fatalf("avatar body = %+v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"URL":"https://pps.whatsapp.net/avatar.jpg"}}`))
	}))
	defer server.Close()

	got, err := New(server.URL, "global-secret", "").AvatarURL(
		context.Background(), "juanse-abc", "5491126437432@s.whatsapp.net",
	)
	if err != nil {
		t.Fatalf("AvatarURL: %v", err)
	}
	if got != "https://pps.whatsapp.net/avatar.jpg" {
		t.Fatalf("avatar URL = %q", got)
	}
}

func TestAvatarURLErrorsWhenEvolutionReturnsNoURL(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"URL":""}}`))
	}))
	defer server.Close()

	if _, err := New(server.URL, "global-secret", "").AvatarURL(
		context.Background(), "juanse-abc", "5491126437432@s.whatsapp.net",
	); err == nil {
		t.Fatal("AvatarURL error = nil, want missing URL error")
	}
}

func TestConfigureWebhookUpdatesSubscriptionsForAnExistingLine(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/instance/connect" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		var body struct {
			WebhookURL string   `json:"webhookUrl"`
			Subscribe  []string `json:"subscribe"`
		}
		decodeRequest(t, r, &body)
		if body.WebhookURL != "http://backend/webhook" || !reflect.DeepEqual(body.Subscribe, subscriptions) {
			t.Fatalf("configuration = %+v", body)
		}
		_, _ = w.Write([]byte(`{"message":"success","data":{}}`))
	}))
	defer server.Close()

	if err := New(server.URL, "global-secret", "http://backend/webhook").ConfigureWebhook(
		context.Background(), "juanse-abc"); err != nil {
		t.Fatalf("ConfigureWebhook: %v", err)
	}
}

func TestCreateInstanceStartsOnlyOneEvolutionClient(t *testing.T) {
	t.Parallel()

	const (
		instanceID   = "249aad2e-68f9-464f-bc84-aca560c38f0e"
		instanceName = "juan-123abc"
		globalKey    = "global-secret"
		webhookURL   = "http://backend:8080/webhooks/evolution"
	)

	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch calls {
		case 1:
			if r.Method != http.MethodPost || r.URL.Path != "/instance/create" {
				t.Fatalf("create request = %s %s", r.Method, r.URL.Path)
			}
			if got := r.Header.Get("apikey"); got != globalKey {
				t.Fatalf("create apikey = %q", got)
			}
			var body struct {
				InstanceID string `json:"instanceId"`
				Name       string `json:"name"`
				Token      string `json:"token"`
			}
			decodeRequest(t, r, &body)
			if body.InstanceID != instanceID || body.Name != instanceName {
				t.Fatalf("create body = %+v", body)
			}
			if body.Token == "" || body.Token == globalKey {
				t.Fatalf("instance token must be non-empty and distinct from global key")
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"message":"success","data":{"id":"` + instanceID + `"}}`))

		case 2:
			if r.Method != http.MethodPost || r.URL.Path != "/instance/connect" {
				t.Fatalf("connect request = %s %s", r.Method, r.URL.Path)
			}
			if got := r.Header.Get("apikey"); got == "" || got == globalKey {
				t.Fatalf("connect must use instance token, got %q", got)
			}
			var body struct {
				WebhookURL string   `json:"webhookUrl"`
				Subscribe  []string `json:"subscribe"`
				Immediate  bool     `json:"immediate"`
			}
			decodeRequest(t, r, &body)
			wantEvents := []string{"MESSAGE", "SEND_MESSAGE", "READ_RECEIPT", "HISTORY_SYNC", "CONNECTION", "CONTACT", "GROUP", "QRCODE"}
			if body.WebhookURL != webhookURL || !body.Immediate || !reflect.DeepEqual(body.Subscribe, wantEvents) {
				t.Fatalf("connect body = %+v", body)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"message":"success","data":{}}`))

		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := New(server.URL, globalKey, webhookURL)
	got, err := client.CreateInstance(context.Background(), instanceID, instanceName)
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	if got != "" {
		t.Fatalf("qr = %q, want async delivery through webhook", got)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want create + connect only", calls)
	}
}

func TestSendTextUsesInstanceTokenAndReadsEvolutionGoMessageID(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/send/text" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("apikey"); got == "" || got == "global-secret" {
			t.Fatalf("send must use instance token, got %q", got)
		}
		var body map[string]any
		decodeRequest(t, r, &body)
		if body["number"] != "5491112345678@s.whatsapp.net" || body["text"] != "hola" {
			t.Fatalf("send body = %#v", body)
		}
		if _, ok := body["quoted"]; ok {
			t.Fatalf("un envío normal no debe incluir quoted: %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":"success","data":{"Info":{"ID":"3EB012345","Chat":"5491112345678@s.whatsapp.net","IsFromMe":true}}}`))
	}))
	defer server.Close()

	client := New(server.URL, "global-secret", "http://backend/webhooks/evolution")
	result, err := client.SendText(context.Background(), "juan-123abc", "5491112345678@s.whatsapp.net", "hola", nil)
	if err != nil {
		t.Fatalf("SendText: %v", err)
	}
	if result.Key.ID != "3EB012345" {
		t.Fatalf("message id = %q", result.Key.ID)
	}
}

func TestSendTextIncludesQuotedMessage(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Number string `json:"number"`
			Text   string `json:"text"`
			Quoted struct {
				MessageID   string `json:"messageId"`
				Participant string `json:"participant"`
			} `json:"quoted"`
		}
		decodeRequest(t, r, &body)
		if body.Quoted.MessageID != "wa-original" ||
			body.Quoted.Participant != "5491112345678@s.whatsapp.net" {
			t.Fatalf("quoted = %+v", body.Quoted)
		}
		_, _ = w.Write([]byte(`{"data":{"Info":{"ID":"wa-reply","Chat":"5491112345678@s.whatsapp.net","IsFromMe":true}}}`))
	}))
	defer server.Close()

	_, err := New(server.URL, "global-secret", "").SendText(
		context.Background(), "juan-123abc", "5491112345678@s.whatsapp.net", "respuesta",
		&Quote{MessageID: "wa-original", Participant: "5491112345678@s.whatsapp.net"},
	)
	if err != nil {
		t.Fatalf("SendText: %v", err)
	}
}

func TestSendMediaIncludesQuotedMessage(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			URL    string `json:"url"`
			Quoted struct {
				MessageID   string `json:"messageId"`
				Participant string `json:"participant"`
			} `json:"quoted"`
		}
		decodeRequest(t, r, &body)
		if body.URL != "https://files.example/pasaporte.pdf" || body.Quoted.MessageID != "wa-original" ||
			body.Quoted.Participant != "5491112345678@s.whatsapp.net" {
			t.Fatalf("media body = %+v", body)
		}
		_, _ = w.Write([]byte(`{"data":{"Info":{"ID":"wa-media-reply","Chat":"5491112345678@s.whatsapp.net","IsFromMe":true}}}`))
	}))
	defer server.Close()

	_, err := New(server.URL, "global-secret", "").SendMedia(
		context.Background(), "juan-123abc", "5491112345678@s.whatsapp.net",
		"https://files.example/pasaporte.pdf", "document", "adjunto", "pasaporte.pdf",
		&Quote{MessageID: "wa-original", Participant: "5491112345678@s.whatsapp.net"},
	)
	if err != nil {
		t.Fatalf("SendMedia: %v", err)
	}
}

func decodeRequest(t *testing.T, r *http.Request, dst any) {
	t.Helper()
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		t.Fatalf("decode request: %v", err)
	}
}
