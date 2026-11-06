// Package menu muestra las reuniones en rofi y abre la elegida.
package menu

import (
	"bytes"
	"errors"
	"fmt"
	"html"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/Allrivenjs/redus/internal/store"
)

// Show abre rofi con las reuniones que no han terminado y abre el enlace de
// la elegida (la videollamada o, si no tiene, el evento en Calendar). La
// primera opción abre la vista del día con openAgenda.
func Show(events []store.Event, accounts []string, now time.Time, openAgenda func() error, open func(link, account string) error) error {
	if len(accounts) == 0 {
		return exec.Command("rofi", "-e", "Sin cuentas conectadas. Usa: redus auth add").Run()
	}
	events = store.Upcoming(events, now)
	multi := len(accounts) > 1
	lines := []string{"<b>\U000F00ED  Ver agenda del día</b>"}
	if len(events) == 0 {
		lines = append(lines, "<i>No hay reuniones próximas</i>")
	}
	for _, e := range events {
		lines = append(lines, row(e, now, multi))
	}
	cmd := exec.Command("rofi", "-dmenu", "-i", "-markup-rows",
		"-p", "Reuniones", "-format", "i", "-no-custom",
		"-mesg", "Enter abre la videollamada (o el evento si no tiene enlace)")
	cmd.Stdin = strings.NewReader(strings.Join(lines, "\n"))
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return nil // cerrado sin elegir
		}
		return err
	}
	i, err := strconv.Atoi(strings.TrimSpace(out.String()))
	if err != nil {
		return nil
	}
	if i == 0 {
		return openAgenda()
	}
	i-- // la fila 0 es la agenda
	if i < 0 || i >= len(events) {
		return nil
	}
	e := events[i]
	link := e.JoinURL
	if link == "" {
		link = e.HTMLLink
	}
	if link == "" {
		return fmt.Errorf("la reunión %q no tiene enlace", e.Title)
	}
	return open(link, e.Account)
}

func row(e store.Event, now time.Time, multi bool) string {
	start := e.Start.Local()
	day := start.Format("02/01")
	switch {
	case sameDay(start, now):
		day = "Hoy"
	case sameDay(start, now.AddDate(0, 0, 1)):
		day = "Mañana"
	}
	when := fmt.Sprintf("%s %s", day, start.Format("15:04"))
	if !e.Start.After(now) {
		when = "En curso"
	}
	s := fmt.Sprintf("<b>%-9s</b>  %s", html.EscapeString(when), html.EscapeString(e.Title))
	if e.JoinURL == "" {
		s += "  <i>(sin enlace)</i>"
	}
	if multi {
		s += "  <small>" + html.EscapeString(e.Account) + "</small>"
	}
	return s
}

func sameDay(a, b time.Time) bool {
	a, b = a.Local(), b.Local()
	return a.Year() == b.Year() && a.YearDay() == b.YearDay()
}
