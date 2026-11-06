// Package waybar arma la salida JSON del módulo custom/redus.
package waybar

import (
	"fmt"
	"html"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Allrivenjs/redus/internal/config"
	"github.com/Allrivenjs/redus/internal/store"
)

// Íconos Nerd Font (Material Design).
const (
	IconCalendar = "\U000F00F0" // md-calendar_clock
	IconMic      = "\U000F036C" // md-microphone
	IconBellOff  = "\U000F009B" // md-bell_off
	IconVideo    = "\U000F0567" // md-video
)

// Output es el formato JSON que espera waybar con "return-type": "json".
type Output struct {
	Text    string   `json:"text"`
	Tooltip string   `json:"tooltip"`
	Class   []string `json:"class"`
	Alt     string   `json:"alt"`
}

// Input reúne todo lo que necesita Render, para poder probarlo sin disco.
type Input struct {
	Now      time.Time
	Cache    store.Cache
	Status   store.Status
	Override store.Override
	Config   config.Config
	// DaemonAlive indica si el daemon está corriendo.
	DaemonAlive bool
}

// Render decide qué mostrar en la barra.
func Render(in Input) Output {
	now := in.Now
	events := store.Upcoming(in.Cache.Events, now)
	var ongoing, next *store.Event
	for i := range events {
		e := &events[i]
		if !e.Start.After(now) {
			if ongoing == nil {
				ongoing = e
			}
			continue
		}
		next = e
		break
	}

	out := Output{Tooltip: tooltip(in, events)}
	var classes []string
	var parts []string

	paused := in.Status.Paused && in.DaemonAlive
	if in.Override == store.OverrideOn {
		classes = append(classes, "dnd")
	}

	untilNext := time.Duration(-1)
	if next != nil {
		untilNext = next.Start.Sub(now)
	}
	soon := func(min int) bool {
		return untilNext >= 0 && untilNext <= time.Duration(min)*time.Minute
	}

	switch {
	case !in.DaemonAlive:
		parts = append(parts, IconCalendar)
		classes = append(classes, "stopped")
	case in.Status.InCall:
		parts = append(parts, IconMic+" En llamada")
		classes = append(classes, "call")
		if soon(in.Config.UrgentMinutes) {
			parts = append(parts, IconCalendar+" "+short(next.Title)+" "+countdown(untilNext))
			classes = append(classes, "urgent")
		}
	case soon(in.Config.UrgentMinutes):
		parts = append(parts, IconCalendar+" "+short(next.Title)+" · "+countdown(untilNext))
		classes = append(classes, "urgent")
	case soon(15):
		parts = append(parts, IconCalendar+" "+short(next.Title)+" · "+countdown(untilNext))
		classes = append(classes, "warning")
	case ongoing != nil:
		parts = append(parts, IconVideo+" "+short(ongoing.Title)+" · en curso")
		classes = append(classes, "ongoing")
	case soon(in.Config.CountdownMinutes):
		parts = append(parts, IconCalendar+" "+short(next.Title)+" · "+countdown(untilNext))
		classes = append(classes, "upcoming")
	case next != nil && sameDay(next.Start, now):
		parts = append(parts, IconCalendar+" "+next.Start.Local().Format("15:04"))
		classes = append(classes, "upcoming")
	case len(in.Cache.Accounts) == 0:
		parts = append(parts, IconCalendar)
		classes = append(classes, "setup")
	default:
		parts = append(parts, IconCalendar)
		classes = append(classes, "idle")
	}
	if paused {
		parts = append(parts, IconBellOff)
		classes = append(classes, "paused")
	}
	if len(in.Cache.Errors) > 0 {
		classes = append(classes, "error")
	}

	out.Text = strings.Join(parts, " · ")
	out.Class = classes
	out.Alt = classes[0]
	return out
}

func tooltip(in Input, events []store.Event) string {
	var b strings.Builder
	now := in.Now
	multi := len(in.Cache.Accounts) > 1

	if !in.DaemonAlive {
		b.WriteString("<b>El daemon de redus no está corriendo.</b>\nsystemctl --user start redus\n\n")
	}
	if len(in.Cache.Accounts) == 0 {
		b.WriteString("Sin cuentas conectadas.\nConecta una con: <tt>redus auth add</tt>\n")
	}

	lastDay := ""
	shown := 0
	for _, e := range events {
		day := dayLabel(e.Start, now)
		if day != lastDay {
			if lastDay != "" {
				b.WriteString("\n")
			}
			fmt.Fprintf(&b, "<b>%s</b>\n", day)
			lastDay = day
		}
		mark := "  "
		if !e.Start.After(now) {
			mark = "▶ "
		}
		fmt.Fprintf(&b, "%s<tt>%s–%s</tt>  %s", mark,
			e.Start.Local().Format("15:04"), e.End.Local().Format("15:04"), esc(e.Title))
		if e.JoinURL == "" {
			b.WriteString("  <span alpha='60%'>(sin enlace)</span>")
		}
		if multi {
			fmt.Fprintf(&b, "  <span alpha='55%%' size='small'>%s</span>", esc(e.Account))
		}
		b.WriteString("\n")
		shown++
		if shown >= 15 {
			b.WriteString("…\n")
			break
		}
	}
	if shown == 0 && len(in.Cache.Accounts) > 0 {
		b.WriteString("No hay reuniones próximas.\n")
	}

	var foot []string
	if in.Status.InCall && len(in.Status.CallApps) > 0 {
		foot = append(foot, IconMic+" Llamada detectada: "+esc(strings.Join(in.Status.CallApps, ", ")))
	}
	if in.Status.Paused && in.DaemonAlive {
		foot = append(foot, IconBellOff+" Notificaciones en pausa (se mostrarán al terminar)")
	}
	if in.Override == store.OverrideOn {
		foot = append(foot, "No molestar activado a mano")
	}
	for acc, err := range in.Cache.Errors {
		foot = append(foot, fmt.Sprintf("<span color='#e06c75'>⚠ %s: %s</span>", esc(acc), esc(truncate(err, 90))))
	}
	if !in.Cache.UpdatedAt.IsZero() {
		foot = append(foot, "<span alpha='55%'>Sincronizado "+in.Cache.UpdatedAt.Local().Format("15:04")+"</span>")
	}
	foot = append(foot, "<span alpha='55%' size='small'>clic: lista · der: no molestar · medio: sincronizar</span>")

	return strings.TrimRight(b.String(), "\n") + "\n\n" + strings.Join(foot, "\n")
}

func dayLabel(t, now time.Time) string {
	t, now = t.Local(), now.Local()
	switch {
	case sameDay(t, now):
		return "Hoy"
	case sameDay(t, now.AddDate(0, 0, 1)):
		return "Mañana"
	}
	return weekdays[t.Weekday()] + " " + t.Format("02/01")
}

var weekdays = [...]string{"Domingo", "Lunes", "Martes", "Miércoles", "Jueves", "Viernes", "Sábado"}

func sameDay(a, b time.Time) bool {
	a, b = a.Local(), b.Local()
	return a.Year() == b.Year() && a.YearDay() == b.YearDay()
}

// countdown formatea el tiempo restante: "ahora", "12m", "1h05".
func countdown(d time.Duration) string {
	m := int((d + 59*time.Second) / time.Minute) // redondeo hacia arriba
	switch {
	case d < 30*time.Second:
		return "ahora"
	case m < 60:
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%dh%02d", m/60, m%60)
}

func short(s string) string { return esc(truncate(s, 22)) }

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:n-1])) + "…"
}

// esc escapa texto para el markup de Pango que usa waybar.
func esc(s string) string { return html.EscapeString(s) }
