// Package config carga la configuración de redus y resuelve las rutas que usa.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Config es lo que el usuario puede ajustar en ~/.config/redus/config.json.
// Todo campo ausente toma el valor de Default().
type Config struct {
	// Minutos antes del inicio de una reunión en que se notifica (uno por aviso).
	RemindMinutes []int `json:"remind_minutes"`
	// Cada cuánto se consulta Google Calendar.
	SyncInterval Duration `json:"sync_interval"`
	// Cada cuánto se revisa si hay una llamada en curso.
	CallPollInterval Duration `json:"call_poll_interval"`
	// Días hacia adelante que se traen del calendario.
	LookaheadDays int `json:"lookahead_days"`
	// Programas cuyo uso del micrófono no cuenta como llamada. Se compara,
	// sin distinguir mayúsculas, contra application.name y process.binary.
	IgnoreApps []string `json:"ignore_apps"`
	// Señal RTMIN+N con la que se refresca el módulo de waybar.
	WaybarSignal int `json:"waybar_signal"`
	// A partir de cuántos minutos antes la barra muestra cuenta regresiva.
	CountdownMinutes int `json:"countdown_minutes"`
	// Minutos antes del inicio en que la barra pasa a estado urgente.
	UrgentMinutes int `json:"urgent_minutes"`
}

// Duration acepta "5m", "30s" en JSON.
type Duration struct{ time.Duration }

func (d Duration) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }

func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("duración inválida %s: usa un texto como \"5m\"", b)
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	d.Duration = v
	return nil
}

func Default() Config {
	return Config{
		RemindMinutes:    []int{10, 1},
		SyncInterval:     Duration{time.Minute},
		CallPollInterval: Duration{3 * time.Second},
		LookaheadDays:    2,
		IgnoreApps: []string{
			"discord", "vesktop", "webcord", "armcord", "legcord",
			"cava", "pavucontrol", "easyeffects", "pwvucontrol",
			"helvum", "qpwgraph", "obs",
		},
		WaybarSignal:     11,
		CountdownMinutes: 60,
		UrgentMinutes:    2,
	}
}

// Load lee el archivo de configuración; si no existe devuelve los valores por defecto.
func Load() (Config, error) {
	c := Default()
	b, err := os.ReadFile(File())
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("%s: %w", File(), err)
	}
	return c, nil
}

func xdg(env, fallback string) string {
	if v := os.Getenv(env); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, fallback)
}

// ConfigDir guarda configuración, credenciales OAuth y tokens.
func ConfigDir() string { return filepath.Join(xdg("XDG_CONFIG_HOME", ".config"), "redus") }

// CacheDir guarda el caché de eventos y el estado que lee la barra.
func CacheDir() string { return filepath.Join(xdg("XDG_CACHE_HOME", ".cache"), "redus") }

// RuntimeDir guarda el pid del daemon.
func RuntimeDir() string {
	if v := os.Getenv("XDG_RUNTIME_DIR"); v != "" {
		return filepath.Join(v, "redus")
	}
	return CacheDir()
}

func File() string            { return filepath.Join(ConfigDir(), "config.json") }
func CredentialsFile() string { return filepath.Join(ConfigDir(), "credentials.json") }
func TokensDir() string       { return filepath.Join(ConfigDir(), "tokens") }
func PIDFile() string         { return filepath.Join(RuntimeDir(), "daemon.pid") }
