package waybar

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Allrivenjs/redus/internal/config"
	"github.com/Allrivenjs/redus/internal/store"
)

func ev(title string, start time.Time, dur time.Duration) store.Event {
	return store.Event{ID: title, Account: "a@x.com", Title: title, Start: start, End: start.Add(dur), JoinURL: "https://meet.google.com/x"}
}

func TestRender(t *testing.T) {
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.Local)
	base := Input{Now: now, Config: config.Default(), DaemonAlive: true,
		Cache: store.Cache{Accounts: []string{"a@x.com"}}}

	cases := []struct {
		name   string
		events []store.Event
		status store.Status
		class  string
		text   string
	}{
		{"sin reuniones", nil, store.Status{}, "idle", IconCalendar},
		{"más de 1h", []store.Event{ev("Revisión", now.Add(3*time.Hour), time.Hour)}, store.Status{}, "upcoming", "13:00"},
		{"cuenta regresiva", []store.Event{ev("Planning", now.Add(40*time.Minute), time.Hour)}, store.Status{}, "upcoming", "40m"},
		{"pronto", []store.Event{ev("Daily", now.Add(12*time.Minute), 15*time.Minute)}, store.Status{}, "warning", "Daily · 12m"},
		{"urgente", []store.Event{ev("Daily", now.Add(20*time.Second), 15*time.Minute)}, store.Status{}, "urgent", "ahora"},
		{"en curso", []store.Event{ev("1:1", now.Add(-10*time.Minute), time.Hour)}, store.Status{}, "ongoing", "en curso"},
		{"en llamada", []store.Event{ev("1:1", now.Add(-10*time.Minute), time.Hour)},
			store.Status{InCall: true, Paused: true, CallApps: []string{"Chrome"}}, "call", "En llamada"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := base
			in.Cache.Events = c.events
			in.Status = c.status
			out := Render(in)
			if !slices.Contains(out.Class, c.class) {
				t.Errorf("clase %v, esperaba %q", out.Class, c.class)
			}
			if !strings.Contains(out.Text, c.text) {
				t.Errorf("texto %q, esperaba que contenga %q", out.Text, c.text)
			}
		})
	}
}

func TestRenderEscapesMarkup(t *testing.T) {
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.Local)
	out := Render(Input{Now: now, Config: config.Default(), DaemonAlive: true,
		Cache: store.Cache{Accounts: []string{"a"}, Events: []store.Event{ev("Q&A <ventas>", now.Add(5*time.Minute), time.Hour)}}})
	if strings.Contains(out.Text, "<ventas>") || strings.Contains(out.Tooltip, "<ventas>") {
		t.Fatalf("el título no se escapó: %q", out.Text)
	}
}

func TestDaemonStopped(t *testing.T) {
	out := Render(Input{Now: time.Now(), Config: config.Default(), DaemonAlive: false,
		Status: store.Status{Paused: true}})
	if !slices.Contains(out.Class, "stopped") || slices.Contains(out.Class, "paused") {
		t.Fatalf("clases %v", out.Class)
	}
}
