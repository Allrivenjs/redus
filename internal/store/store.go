// Package store guarda en disco lo que el daemon sabe (eventos y estado de
// llamada) para que la barra y el menú lo lean sin consultar a Google.
package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Allrivenjs/redus/internal/config"
)

// Event es una reunión normalizada, venga de la cuenta que venga.
type Event struct {
	ID       string    `json:"id"`
	Account  string    `json:"account"`
	Calendar string    `json:"calendar,omitempty"`
	Title    string    `json:"title"`
	Start    time.Time `json:"start"`
	End      time.Time `json:"end"`
	AllDay   bool      `json:"all_day,omitempty"`
	JoinURL  string    `json:"join_url,omitempty"`
	HTMLLink string    `json:"html_link,omitempty"`
	Location string    `json:"location,omitempty"`
}

// Key identifica un evento entre cuentas: la misma reunión puede estar en dos.
func (e Event) Key() string { return e.Account + "/" + e.ID }

// Cache es el resultado de la última sincronización con los calendarios.
type Cache struct {
	UpdatedAt time.Time         `json:"updated_at"`
	Events    []Event           `json:"events"`
	Accounts  []string          `json:"accounts"`
	Errors    map[string]string `json:"errors,omitempty"` // cuenta -> último error
}

// Status es el estado vivo del daemon.
type Status struct {
	UpdatedAt time.Time `json:"updated_at"`
	InCall    bool      `json:"in_call"`
	CallApps  []string  `json:"call_apps,omitempty"`
	Paused    bool      `json:"paused"`
}

// Override es el modo no molestar elegido a mano.
type Override string

const (
	OverrideAuto Override = "auto" // pausa solo durante llamadas
	OverrideOn   Override = "on"   // pausa siempre
)

func cacheFile() string    { return filepath.Join(config.CacheDir(), "events.json") }
func statusFile() string   { return filepath.Join(config.CacheDir(), "status.json") }
func overrideFile() string { return filepath.Join(config.CacheDir(), "dnd") }
func notifiedFile() string { return filepath.Join(config.CacheDir(), "notified.json") }

func LoadCache() (Cache, error) {
	var c Cache
	err := readJSON(cacheFile(), &c)
	return c, err
}

func SaveCache(c Cache) error {
	sort.Slice(c.Events, func(i, j int) bool { return c.Events[i].Start.Before(c.Events[j].Start) })
	return writeJSON(cacheFile(), c)
}

func LoadStatus() (Status, error) {
	var s Status
	err := readJSON(statusFile(), &s)
	return s, err
}

func SaveStatus(s Status) error { return writeJSON(statusFile(), s) }

func LoadOverride() Override {
	b, err := os.ReadFile(overrideFile())
	if err != nil {
		return OverrideAuto
	}
	if Override(strings.TrimSpace(string(b))) == OverrideOn {
		return OverrideOn
	}
	return OverrideAuto
}

func SaveOverride(o Override) error {
	return writeFile(overrideFile(), []byte(string(o)+"\n"))
}

// LoadNotified devuelve los avisos ya enviados (clave -> hora de envío).
func LoadNotified() map[string]time.Time {
	m := map[string]time.Time{}
	_ = readJSON(notifiedFile(), &m)
	return m
}

func SaveNotified(m map[string]time.Time) error { return writeJSON(notifiedFile(), m) }

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(path, b)
}

// writeFile escribe de forma atómica para que la barra nunca lea un archivo a medias.
func writeFile(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Upcoming devuelve los eventos con hora (no de día completo) que no han terminado.
func Upcoming(events []Event, now time.Time) []Event {
	var res []Event
	for _, e := range events {
		if e.AllDay || !e.End.After(now) {
			continue
		}
		res = append(res, e)
	}
	return res
}
