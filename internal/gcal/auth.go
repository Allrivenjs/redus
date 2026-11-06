// Package gcal conecta cuentas de Google por OAuth y lee sus calendarios.
package gcal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"

	"github.com/Allrivenjs/redus/internal/config"
)

// Scopes pedidos al conectar una cuenta. Solo lectura.
var Scopes = []string{
	"https://www.googleapis.com/auth/calendar.readonly",
}

var googleEndpoint = oauth2.Endpoint{
	AuthURL:   "https://accounts.google.com/o/oauth2/auth",
	TokenURL:  "https://oauth2.googleapis.com/token",
	AuthStyle: oauth2.AuthStyleInParams,
}

// ErrNoCredentials indica que falta el cliente OAuth descargado de Google Cloud.
var ErrNoCredentials = errors.New("falta " + config.CredentialsFile() +
	": crea un cliente OAuth de tipo \"App de escritorio\" en Google Cloud y guarda ahí el JSON")

// oauthConfig lee el JSON del cliente OAuth ("installed" o "web").
func oauthConfig() (*oauth2.Config, error) {
	b, err := os.ReadFile(config.CredentialsFile())
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNoCredentials
	}
	if err != nil {
		return nil, err
	}
	var f struct {
		Installed *clientJSON `json:"installed"`
		Web       *clientJSON `json:"web"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", config.CredentialsFile(), err)
	}
	c := f.Installed
	if c == nil {
		c = f.Web
	}
	if c == nil || c.ClientID == "" {
		return nil, fmt.Errorf("%s no parece un cliente OAuth de Google", config.CredentialsFile())
	}
	return &oauth2.Config{
		ClientID:     c.ClientID,
		ClientSecret: c.ClientSecret,
		Endpoint:     googleEndpoint,
		Scopes:       Scopes,
	}, nil
}

type clientJSON struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

// AddAccount abre el navegador para autorizar una cuenta, guarda el token y
// devuelve el correo de la cuenta conectada. hint sugiere la cuenta a Google.
func AddAccount(ctx context.Context, hint string) (string, error) {
	cfg, err := oauthConfig()
	if err != nil {
		return "", err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer ln.Close()
	cfg.RedirectURL = fmt.Sprintf("http://127.0.0.1:%d/", ln.Addr().(*net.TCPAddr).Port)

	state := oauth2.GenerateVerifier()
	verifier := oauth2.GenerateVerifier()
	opts := []oauth2.AuthCodeOption{
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("prompt", "consent select_account"),
		oauth2.S256ChallengeOption(verifier),
	}
	if hint != "" {
		opts = append(opts, oauth2.SetAuthURLParam("login_hint", hint))
	}
	authURL := cfg.AuthCodeURL(state, opts...)

	type result struct {
		code string
		err  error
	}
	done := make(chan result, 1)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("state") != state {
			http.Error(w, "estado inválido", http.StatusBadRequest)
			return
		}
		if e := q.Get("error"); e != "" {
			fmt.Fprintf(w, "Autorización cancelada: %s. Puedes cerrar esta pestaña.", e)
			done <- result{err: fmt.Errorf("google respondió: %s", e)}
			return
		}
		fmt.Fprint(w, "Cuenta conectada a redus. Puedes cerrar esta pestaña.")
		done <- result{code: q.Get("code")}
	})}
	go srv.Serve(ln)
	defer srv.Close()

	fmt.Println("Abriendo el navegador para autorizar la cuenta. Si no se abre, entra a:")
	fmt.Println(authURL)
	_ = exec.Command("xdg-open", authURL).Start()

	var res result
	select {
	case res = <-done:
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(5 * time.Minute):
		return "", errors.New("se agotó el tiempo esperando la autorización")
	}
	if res.err != nil {
		return "", res.err
	}
	tok, err := cfg.Exchange(ctx, res.code, oauth2.VerifierOption(verifier))
	if err != nil {
		return "", fmt.Errorf("canjeando el código: %w", err)
	}
	if tok.RefreshToken == "" {
		return "", errors.New("google no entregó refresh token; quita el acceso de la app en myaccount.google.com/permissions y reintenta")
	}
	cl := &Client{http: cfg.Client(ctx, tok)}
	email, err := cl.PrimaryID(ctx)
	if err != nil {
		return "", fmt.Errorf("leyendo el calendario principal: %w", err)
	}
	if err := saveToken(email, tok); err != nil {
		return "", err
	}
	return email, nil
}

// Accounts lista las cuentas conectadas.
func Accounts() ([]string, error) {
	entries, err := os.ReadDir(config.TokensDir())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var res []string
	for _, e := range entries {
		if n := e.Name(); strings.HasSuffix(n, ".json") && !e.IsDir() {
			res = append(res, strings.TrimSuffix(n, ".json"))
		}
	}
	sort.Strings(res)
	return res, nil
}

// RemoveAccount borra el token local de una cuenta.
func RemoveAccount(email string) error {
	err := os.Remove(tokenPath(email))
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("la cuenta %s no está conectada", email)
	}
	return err
}

// NewClient arma un cliente HTTP autenticado que guarda el token al renovarse.
func NewClient(ctx context.Context, email string) (*Client, error) {
	cfg, err := oauthConfig()
	if err != nil {
		return nil, err
	}
	tok, err := loadToken(email)
	if err != nil {
		return nil, err
	}
	ts := &savingSource{email: email, last: tok, src: cfg.TokenSource(ctx, tok)}
	return &Client{http: oauth2.NewClient(ctx, ts), Account: email}, nil
}

// savingSource persiste el token cada vez que oauth2 lo renueva.
type savingSource struct {
	mu    sync.Mutex
	email string
	last  *oauth2.Token
	src   oauth2.TokenSource
}

func (s *savingSource) Token() (*oauth2.Token, error) {
	tok, err := s.src.Token()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.last == nil || tok.AccessToken != s.last.AccessToken {
		if tok.RefreshToken == "" && s.last != nil {
			tok.RefreshToken = s.last.RefreshToken
		}
		_ = saveToken(s.email, tok)
		s.last = tok
	}
	return tok, nil
}

func tokenPath(email string) string {
	return filepath.Join(config.TokensDir(), filepath.Base(email)+".json")
}

func loadToken(email string) (*oauth2.Token, error) {
	b, err := os.ReadFile(tokenPath(email))
	if err != nil {
		return nil, err
	}
	var t oauth2.Token
	return &t, json.Unmarshal(b, &t)
}

func saveToken(email string, t *oauth2.Token) error {
	if err := os.MkdirAll(config.TokensDir(), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	tmp := tokenPath(email) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, tokenPath(email))
}
