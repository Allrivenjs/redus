// redus: avisos de reuniones de varias cuentas de Google, indicador en waybar
// y modo no molestar automático durante llamadas.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Allrivenjs/redus/internal/calls"
	"github.com/Allrivenjs/redus/internal/config"
	"github.com/Allrivenjs/redus/internal/daemon"
	"github.com/Allrivenjs/redus/internal/gcal"
	"github.com/Allrivenjs/redus/internal/menu"
	"github.com/Allrivenjs/redus/internal/store"
	"github.com/Allrivenjs/redus/internal/waybar"
)

const usage = `redus: avisos de reuniones, indicador en waybar y no molestar en llamadas.

Uso:
  redus daemon              corre el servicio (lo lanza systemd)
  redus waybar              imprime el JSON del módulo de waybar
  redus menu                lista las reuniones en rofi y abre la elegida
  redus sync                sincroniza los calendarios ahora
  redus dnd [toggle|on|auto|status]
                            no molestar manual (auto = solo en llamadas)
  redus auth add [correo]   conecta una cuenta de Google
  redus auth list           lista las cuentas conectadas
  redus auth remove <correo>
  redus status              diagnóstico: llamada, cuentas, próximas reuniones
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "redus:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		fmt.Print(usage)
		return nil
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx := context.Background()

	switch args[0] {
	case "daemon":
		return daemon.Run(ctx, cfg)
	case "waybar":
		return printWaybar(cfg)
	case "menu":
		c, _ := store.LoadCache()
		return menu.Show(c.Events, c.Accounts, time.Now())
	case "sync":
		if !daemon.Signal(daemon.SigSync) {
			return fmt.Errorf("el daemon no está corriendo: systemctl --user start redus")
		}
		return nil
	case "dnd":
		return dndCmd(cfg, args[1:])
	case "auth":
		return authCmd(ctx, args[1:])
	case "status":
		return statusCmd(ctx, cfg)
	case "help", "-h", "--help":
		fmt.Print(usage)
		return nil
	}
	return fmt.Errorf("comando desconocido %q\n\n%s", args[0], usage)
}

func printWaybar(cfg config.Config) error {
	c, _ := store.LoadCache()
	s, _ := store.LoadStatus()
	_, alive := daemon.PID()
	out := waybar.Render(waybar.Input{
		Now:         time.Now(),
		Cache:       c,
		Status:      s,
		Override:    store.LoadOverride(),
		Config:      cfg,
		DaemonAlive: alive,
	})
	return json.NewEncoder(os.Stdout).Encode(out)
}

func dndCmd(cfg config.Config, args []string) error {
	cur := store.LoadOverride()
	action := "status"
	if len(args) > 0 {
		action = args[0]
	}
	next := cur
	switch action {
	case "status":
		fmt.Println(cur)
		return nil
	case "toggle":
		next = store.OverrideOn
		if cur == store.OverrideOn {
			next = store.OverrideAuto
		}
	case "on":
		next = store.OverrideOn
	case "auto", "off":
		next = store.OverrideAuto
	default:
		return fmt.Errorf("uso: redus dnd [toggle|on|auto|status]")
	}
	if err := store.SaveOverride(next); err != nil {
		return err
	}
	if !daemon.Signal(daemon.SigDND) {
		fmt.Fprintln(os.Stderr, "aviso: el daemon no está corriendo; se aplicará cuando arranque")
	}
	daemon.RefreshWaybar(cfg)
	fmt.Println(next)
	return nil
}

func authCmd(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("uso: redus auth add [correo] | list | remove <correo>")
	}
	switch args[0] {
	case "add":
		hint := ""
		if len(args) > 1 {
			hint = args[1]
		}
		email, err := gcal.AddAccount(ctx, hint)
		if err != nil {
			return err
		}
		fmt.Printf("Cuenta conectada: %s\n", email)
		daemon.Signal(daemon.SigSync)
		return nil
	case "list":
		accs, err := gcal.Accounts()
		if err != nil {
			return err
		}
		if len(accs) == 0 {
			fmt.Println("Sin cuentas conectadas. Usa: redus auth add")
		}
		for _, a := range accs {
			fmt.Println(a)
		}
		return nil
	case "remove":
		if len(args) < 2 {
			return fmt.Errorf("uso: redus auth remove <correo>")
		}
		if err := gcal.RemoveAccount(args[1]); err != nil {
			return err
		}
		fmt.Printf("Cuenta quitada: %s (el acceso se revoca en myaccount.google.com/permissions)\n", args[1])
		daemon.Signal(daemon.SigSync)
		return nil
	}
	return fmt.Errorf("uso: redus auth add [correo] | list | remove <correo>")
}

func statusCmd(ctx context.Context, cfg config.Config) error {
	if pid, ok := daemon.PID(); ok {
		fmt.Printf("Daemon: corriendo (pid %d)\n", pid)
	} else {
		fmt.Println("Daemon: detenido")
	}
	streams, err := calls.Detector{Ignore: cfg.IgnoreApps}.Active(ctx)
	if err != nil {
		fmt.Printf("Llamada: error al detectar: %v\n", err)
	} else if len(streams) == 0 {
		fmt.Println("Llamada: no")
	} else {
		var l []string
		for _, s := range streams {
			l = append(l, fmt.Sprintf("%s (%s)", s.Label(), s.Binary))
		}
		fmt.Printf("Llamada: sí — %s\n", strings.Join(l, ", "))
	}
	fmt.Printf("No molestar manual: %s\n", store.LoadOverride())

	c, _ := store.LoadCache()
	fmt.Printf("Cuentas: %d", len(c.Accounts))
	if len(c.Accounts) > 0 {
		fmt.Printf(" (%s)", strings.Join(c.Accounts, ", "))
	}
	fmt.Println()
	for acc, e := range c.Errors {
		fmt.Printf("  ⚠ %s: %s\n", acc, e)
	}
	if !c.UpdatedAt.IsZero() {
		fmt.Printf("Última sincronización: %s\n", c.UpdatedAt.Local().Format("02/01 15:04:05"))
	}
	up := store.Upcoming(c.Events, time.Now())
	if len(up) > 5 {
		up = up[:5]
	}
	for _, e := range up {
		fmt.Printf("  %s  %s  [%s]\n", e.Start.Local().Format("02/01 15:04"), e.Title, e.Account)
	}
	return nil
}
