// Package notify envía notificaciones de escritorio con notify-send.
package notify

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

type Urgency string

const (
	Normal   Urgency = "normal"
	Critical Urgency = "critical"
)

// Message es una notificación. Si JoinURL no está vacío se agrega el botón
// "Unirse", que abre el enlace.
type Message struct {
	Title   string
	Body    string
	Urgency Urgency
	JoinURL string
	Account string // cuenta de la reunión, para abrirla en su perfil
	Icon    string
}

// OpenFunc abre un enlace para una cuenta.
type OpenFunc func(link, account string) error

// Send muestra la notificación sin bloquear. Si lleva botón, espera en
// segundo plano la respuesta (hasta 30 min, porque con dunst en pausa la
// notificación puede quedar en cola).
func Send(m Message, open OpenFunc) error {
	args := []string{"--app-name=redus", "--urgency=" + string(orDefault(m.Urgency))}
	if m.Icon != "" {
		args = append(args, "--icon="+m.Icon)
	}
	if m.JoinURL == "" {
		args = append(args, m.Title, m.Body)
		return exec.Command("notify-send", args...).Run()
	}
	args = append(args, "--action=join=Unirse", "--wait", m.Title, m.Body)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		out, err := exec.CommandContext(ctx, "notify-send", args...).Output()
		if err == nil && strings.TrimSpace(string(out)) == "join" {
			_ = open(m.JoinURL, m.Account)
		}
	}()
	return nil
}

func orDefault(u Urgency) Urgency {
	if u == "" {
		return Normal
	}
	return u
}
