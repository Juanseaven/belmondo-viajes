package fuentes

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const passportIndexFile = "passport-index-tidy-iso2.csv"

// PassportIndex descarga la matriz que publica imorte/passport-index-data
// (MIT) en GitHub. No se accede a passportindex.org.
type PassportIndex struct {
	RawURL     string
	CommitsURL string
	HTTP       *http.Client
}

func NewPassportIndex() *PassportIndex {
	return &PassportIndex{
		RawURL:     "https://raw.githubusercontent.com/imorte/passport-index-data/main/" + passportIndexFile,
		CommitsURL: "https://api.github.com/repos/imorte/passport-index-data/commits",
		HTTP:       &http.Client{Timeout: 60 * time.Second},
	}
}

// PassportIndexSnapshot es la matriz descargada, por [pasaporte, destino], y
// la fecha del último cambio del archivo, que es la fecha de sus datos.
type PassportIndexSnapshot struct {
	Entries  map[[2]string]string
	DataDate string
}

func (p *PassportIndex) Download(ctx context.Context) (PassportIndexSnapshot, error) {
	body, err := p.get(ctx, p.RawURL)
	if err != nil {
		return PassportIndexSnapshot{}, err
	}
	defer body.Close()
	entries, err := ParsePassportIndex(body)
	if err != nil {
		return PassportIndexSnapshot{}, err
	}

	commits, err := p.get(ctx, p.CommitsURL+"?"+url.Values{"path": {passportIndexFile}, "per_page": {"1"}}.Encode())
	if err != nil {
		return PassportIndexSnapshot{}, err
	}
	defer commits.Close()
	var history []struct {
		Commit struct {
			Committer struct {
				Date time.Time `json:"date"`
			} `json:"committer"`
		} `json:"commit"`
	}
	if err := json.NewDecoder(commits).Decode(&history); err != nil || len(history) == 0 {
		return PassportIndexSnapshot{}, fmt.Errorf("leyendo la fecha de Passport Index: %v", err)
	}
	return PassportIndexSnapshot{Entries: entries, DataDate: history[0].Commit.Committer.Date.Format("2006-01-02")}, nil
}

func (p *PassportIndex) get(ctx context.Context, target string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "viajes-crm/1.0")
	resp, err := p.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("descargando Passport Index: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("descargando Passport Index: status %d", resp.StatusCode)
	}
	return resp.Body, nil
}

// ParsePassportIndex lee la matriz en formato tidy: pasaporte, destino y
// requisito, con encabezado.
func ParsePassportIndex(r io.Reader) (map[[2]string]string, error) {
	records, err := csv.NewReader(r).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("leyendo Passport Index: %w", err)
	}
	if len(records) == 0 || len(records[0]) != 3 || records[0][0] != "Passport" {
		return nil, fmt.Errorf("Passport Index: encabezado inesperado")
	}
	out := make(map[[2]string]string, len(records)-1)
	for _, rec := range records[1:] {
		if len(rec) == 3 {
			out[[2]string{rec[0], rec[1]}] = rec[2]
		}
	}
	return out, nil
}
