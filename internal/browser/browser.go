// Package browser abre enlaces de reuniones en el perfil de Chrome que
// corresponde a la cuenta de la reunión.
package browser

import (
	"encoding/json"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Opener sabe qué navegador usar y qué perfil corresponde a cada cuenta.
type Opener struct {
	// Browser es el ejecutable de Chrome/Chromium.
	Browser string
	// Profiles fuerza el perfil de una cuenta (correo -> "Profile 3"). Las
	// cuentas que no estén aquí se buscan en el Local State de Chrome.
	Profiles map[string]string
	// UserDataDir es la carpeta de datos de Chrome; vacío = la de Browser.
	UserDataDir string
}

// Open abre link para la cuenta account. Si encuentra el perfil de Chrome de
// esa cuenta lo abre ahí; si no, con el navegador por defecto.
func (o Opener) Open(link, account string) error {
	link = WithAuthUser(link, account)
	if account != "" && o.Browser != "" {
		if _, err := exec.LookPath(o.Browser); err == nil {
			if p := o.ProfileFor(account); p != "" {
				return start(o.Browser, "--profile-directory="+p, link)
			}
		}
	}
	return start("xdg-open", link)
}

// ProfileFor devuelve la carpeta del perfil de Chrome con esa cuenta.
func (o Opener) ProfileFor(account string) string {
	account = strings.ToLower(strings.TrimSpace(account))
	for k, v := range o.Profiles {
		if strings.ToLower(k) == account {
			return v
		}
	}
	b, err := os.ReadFile(filepath.Join(o.dataDir(), "Local State"))
	if err != nil {
		return ""
	}
	var st struct {
		Profile struct {
			InfoCache map[string]struct {
				UserName string `json:"user_name"`
			} `json:"info_cache"`
		} `json:"profile"`
	}
	if json.Unmarshal(b, &st) != nil {
		return ""
	}
	for dir, info := range st.Profile.InfoCache {
		if strings.ToLower(info.UserName) == account {
			return dir
		}
	}
	return ""
}

func (o Opener) dataDir() string {
	if o.UserDataDir != "" {
		return o.UserDataDir
	}
	cfg := os.Getenv("XDG_CONFIG_HOME")
	if cfg == "" {
		home, _ := os.UserHomeDir()
		cfg = filepath.Join(home, ".config")
	}
	name := "google-chrome"
	switch b := filepath.Base(o.Browser); {
	case strings.Contains(b, "chromium"):
		name = "chromium"
	case strings.Contains(b, "brave"):
		name = filepath.Join("BraveSoftware", "Brave-Browser")
	case strings.Contains(b, "beta"):
		name = "google-chrome-beta"
	case strings.Contains(b, "unstable"):
		name = "google-chrome-unstable"
	}
	return filepath.Join(cfg, name)
}

// WithAuthUser agrega authuser=<correo> a los enlaces de Meet y Calendar,
// para que Google elija esa cuenta aunque el perfil tenga varias sesiones.
func WithAuthUser(link, account string) string {
	if account == "" {
		return link
	}
	u, err := url.Parse(link)
	if err != nil {
		return link
	}
	switch u.Host {
	case "meet.google.com", "calendar.google.com", "www.google.com":
	default:
		return link
	}
	q := u.Query()
	if q.Get("authuser") != "" {
		return link
	}
	q.Set("authuser", account)
	u.RawQuery = q.Encode()
	return u.String()
}

// Scheme es el protocolo con que la vista de agenda le pide a redus abrir un
// enlace: redus://open?url=<enlace>&account=<correo>.
const Scheme = "redus"

// SchemeURL arma un enlace redus:// para usar dentro de la agenda.
func SchemeURL(link, account string) string {
	q := url.Values{"url": {link}}
	if account != "" {
		q.Set("account", account)
	}
	return Scheme + "://open?" + q.Encode()
}

// ParseSchemeURL extrae el enlace y la cuenta de un redus://open?...
// Solo acepta destinos http(s), para que una página no pueda pedir abrir
// otra cosa a través del protocolo.
func ParseSchemeURL(raw string) (link, account string, ok bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != Scheme || u.Host != "open" {
		return "", "", false
	}
	link = u.Query().Get("url")
	t, err := url.Parse(link)
	if err != nil || (t.Scheme != "https" && t.Scheme != "http") || t.Host == "" {
		return "", "", false
	}
	return link, u.Query().Get("account"), true
}

func start(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
