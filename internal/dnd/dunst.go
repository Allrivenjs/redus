// Package dnd pausa y reanuda las notificaciones de dunst.
//
// Con dunst en pausa las notificaciones no se pierden: quedan en cola y se
// muestran al reanudar.
package dnd

import (
	"context"
	"os/exec"
	"strings"
)

func Paused(ctx context.Context) (bool, error) {
	out, err := exec.CommandContext(ctx, "dunstctl", "is-paused").Output()
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(out)) == "true", nil
}

func SetPaused(ctx context.Context, paused bool) error {
	v := "false"
	if paused {
		v = "true"
	}
	return exec.CommandContext(ctx, "dunstctl", "set-paused", v).Run()
}
