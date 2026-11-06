// Package daemon es el proceso que corre siempre: detecta llamadas, pausa
// dunst, sincroniza calendarios y envía los avisos de reunión.
package daemon

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Allrivenjs/redus/internal/calls"
	"github.com/Allrivenjs/redus/internal/config"
	"github.com/Allrivenjs/redus/internal/dnd"
	"github.com/Allrivenjs/redus/internal/gcal"
	"github.com/Allrivenjs/redus/internal/notify"
	"github.com/Allrivenjs/redus/internal/store"
)

// Signals que entiende el daemon.
const (
	SigSync = syscall.SIGUSR1 // sincronizar calendarios ya
	SigDND  = syscall.SIGUSR2 // releer el modo no molestar
)

type daemon struct {
	cfg      config.Config
	detector calls.Detector
	status   store.Status
	// ownPause indica que la pausa actual de dunst la puso redus; solo así
	// la quita. Si la pausó el usuario por otro medio, se respeta.
	ownPause  bool
	lastWrite time.Time
	notified  map[string]time.Time
	// sendFn envía el aviso; se reemplaza en los tests.
	sendFn func(store.Event, time.Time)
}

// Run corre el daemon hasta recibir SIGINT/SIGTERM.
func Run(ctx context.Context, cfg config.Config) error {
	if pid, ok := PID(); ok {
		return fmt.Errorf("ya hay un daemon corriendo (pid %d)", pid)
	}
	if err := writePID(); err != nil {
		return err
	}
	defer os.Remove(config.PIDFile())

	d := &daemon{
		cfg:      cfg,
		detector: calls.Detector{Ignore: cfg.IgnoreApps},
		notified: store.LoadNotified(),
	}
	d.sendFn = d.send
	// Retoma el estado anterior: si redus dejó dunst en pausa (p. ej. se
	// cayó durante una llamada), se considera propia.
	if prev, err := store.LoadStatus(); err == nil && prev.Paused {
		if p, err := dnd.Paused(ctx); err == nil && p {
			d.ownPause = true
		}
	}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	sigs := make(chan os.Signal, 4)
	signal.Notify(sigs, SigSync, SigDND)
	defer signal.Stop(sigs)

	log.Printf("redus daemon iniciado (llamadas cada %s, calendario cada %s)",
		cfg.CallPollInterval.Duration, cfg.SyncInterval.Duration)

	d.checkCall(ctx)
	d.sync(ctx)
	d.remind(time.Now())

	callT := time.NewTicker(cfg.CallPollInterval.Duration)
	syncT := time.NewTicker(cfg.SyncInterval.Duration)
	remindT := time.NewTicker(15 * time.Second)
	defer callT.Stop()
	defer syncT.Stop()
	defer remindT.Stop()

	for {
		select {
		case <-ctx.Done():
			d.shutdown()
			return nil
		case s := <-sigs:
			switch s {
			case SigSync:
				d.sync(ctx)
				d.remind(time.Now())
			case SigDND:
				d.checkCall(ctx)
			}
		case <-callT.C:
			d.checkCall(ctx)
		case <-syncT.C:
			d.sync(ctx)
		case now := <-remindT.C:
			d.remind(now)
		}
	}
}

func (d *daemon) checkCall(ctx context.Context) {
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	streams, err := d.detector.Active(cctx)
	if err != nil {
		log.Printf("detectando llamada: %v", err)
		return // mantiene el último estado conocido
	}
	var apps []string
	for _, s := range streams {
		if l := s.Label(); !slices.Contains(apps, l) {
			apps = append(apps, l)
		}
	}
	inCall := len(apps) > 0
	want := inCall || store.LoadOverride() == store.OverrideOn

	paused, err := dnd.Paused(cctx)
	if err != nil {
		log.Printf("consultando dunst: %v", err)
	}
	switch {
	case want && !paused:
		if err := dnd.SetPaused(cctx, true); err != nil {
			log.Printf("pausando dunst: %v", err)
		} else {
			paused, d.ownPause = true, true
			if inCall {
				log.Printf("notificaciones en pausa (llamada: %s)", strings.Join(apps, ", "))
			} else {
				log.Printf("notificaciones en pausa (no molestar manual)")
			}
		}
	case !want && paused && d.ownPause:
		if err := dnd.SetPaused(cctx, false); err != nil {
			log.Printf("reanudando dunst: %v", err)
		} else {
			paused, d.ownPause = false, false
			log.Printf("notificaciones reanudadas")
		}
	case !paused:
		d.ownPause = false
	}

	next := store.Status{InCall: inCall, CallApps: apps, Paused: paused}
	changed := next.InCall != d.status.InCall || next.Paused != d.status.Paused ||
		!slices.Equal(next.CallApps, d.status.CallApps)
	if changed || time.Since(d.lastWrite) > time.Minute {
		next.UpdatedAt = time.Now()
		d.status = next
		if err := store.SaveStatus(next); err != nil {
			log.Printf("guardando estado: %v", err)
		}
		d.lastWrite = time.Now()
	}
	if changed {
		RefreshWaybar(d.cfg)
	}
}

func (d *daemon) sync(ctx context.Context) {
	accounts, err := gcal.Accounts()
	if err != nil {
		log.Printf("listando cuentas: %v", err)
		return
	}
	prev, _ := store.LoadCache()
	cache := store.Cache{UpdatedAt: time.Now(), Accounts: accounts, Errors: map[string]string{}}
	now := time.Now()
	from := now.Add(-12 * time.Hour)
	to := startOfDay(now).AddDate(0, 0, d.cfg.LookaheadDays+1)

	for _, acc := range accounts {
		evs, err := d.fetch(ctx, acc, from, to)
		if err != nil {
			log.Printf("sincronizando %s: %v", acc, err)
			cache.Errors[acc] = friendlyError(err)
			// conserva lo último que se supo de esa cuenta
			for _, e := range prev.Events {
				if e.Account == acc {
					cache.Events = append(cache.Events, e)
				}
			}
			continue
		}
		cache.Events = append(cache.Events, evs...)
	}
	if len(cache.Errors) == 0 {
		cache.Errors = nil
	}
	if err := store.SaveCache(cache); err != nil {
		log.Printf("guardando caché: %v", err)
		return
	}
	RefreshWaybar(d.cfg)
}

func (d *daemon) fetch(ctx context.Context, acc string, from, to time.Time) ([]store.Event, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cl, err := gcal.NewClient(ctx, acc)
	if err != nil {
		return nil, err
	}
	return cl.Events(ctx, from, to)
}

func friendlyError(err error) string {
	s := err.Error()
	switch {
	case errors.Is(err, gcal.ErrNoCredentials):
		return "faltan las credenciales OAuth"
	case strings.Contains(s, "invalid_grant"):
		return "el acceso venció o fue revocado: redus auth add"
	case strings.Contains(s, "no such host"), strings.Contains(s, "network is unreachable"):
		return "sin conexión"
	}
	return s
}

// remind envía los avisos que tocan. Si varios umbrales ya pasaron (p. ej.
// al arrancar el PC 3 min antes de una reunión), manda solo el más cercano.
// Una reunión en curso de la que no se avisó (el PC estaba apagado) recibe un
// aviso de "empezó hace N min" mientras no haya terminado.
func (d *daemon) remind(now time.Time) {
	cache, err := store.LoadCache()
	if err != nil {
		return
	}
	mins := slices.Clone(d.cfg.RemindMinutes)
	slices.Sort(mins) // de menor a mayor
	dirty := false
	sent := map[string]bool{} // la misma reunión en dos cuentas avisa una vez

	for _, e := range store.Upcoming(cache.Events, now) {
		for _, m := range mins {
			key := fmt.Sprintf("%s@%d/%d", e.Key(), e.Start.Unix(), m)
			if _, done := d.notified[key]; done {
				continue
			}
			if now.Before(e.Start.Add(-time.Duration(m) * time.Minute)) {
				continue
			}
			// toca este umbral: se marcan también los mayores sin avisar
			for _, bigger := range mins {
				if bigger >= m {
					d.notified[fmt.Sprintf("%s@%d/%d", e.Key(), e.Start.Unix(), bigger)] = now
				}
			}
			dirty = true
			dup := e.Title + "@" + strconv.FormatInt(e.Start.Unix(), 10)
			if !sent[dup] {
				sent[dup] = true
				d.sendFn(e, now)
			}
			break
		}
	}
	for k, t := range d.notified {
		if now.Sub(t) > 72*time.Hour {
			delete(d.notified, k)
			dirty = true
		}
	}
	if dirty {
		if err := store.SaveNotified(d.notified); err != nil {
			log.Printf("guardando avisos: %v", err)
		}
	}
}

func (d *daemon) send(e store.Event, now time.Time) {
	left := e.Start.Sub(now)
	when := "Empieza ahora"
	switch {
	case left >= time.Minute:
		when = fmt.Sprintf("Empieza en %d min", int((left+30*time.Second)/time.Minute))
	case left <= -time.Minute:
		when = "En curso: empezó hace " + ago(-left)
	}
	body := fmt.Sprintf("%s · %s–%s", when, e.Start.Local().Format("15:04"), e.End.Local().Format("15:04"))
	if e.Account != "" {
		body += "\n" + e.Account
	}
	u := notify.Normal
	if left < 2*time.Minute {
		u = notify.Critical
	}
	err := notify.Send(notify.Message{
		Title:   e.Title,
		Body:    body,
		Urgency: u,
		JoinURL: e.JoinURL,
		Icon:    "appointment-soon",
	})
	if err != nil {
		log.Printf("notificando %q: %v", e.Title, err)
	}
}

func (d *daemon) shutdown() {
	if d.ownPause {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := dnd.SetPaused(ctx, false); err == nil {
			d.status.Paused = false
		}
	}
	d.status.InCall = false
	d.status.CallApps = nil
	d.status.UpdatedAt = time.Now()
	_ = store.SaveStatus(d.status)
	os.Remove(config.PIDFile())
	RefreshWaybar(d.cfg)
	log.Printf("redus daemon detenido")
}

func ago(d time.Duration) string {
	m := int(d / time.Minute)
	if m < 60 {
		return fmt.Sprintf("%d min", m)
	}
	return fmt.Sprintf("%d h %02d min", m/60, m%60)
}

func startOfDay(t time.Time) time.Time {
	y, m, dd := t.Date()
	return time.Date(y, m, dd, 0, 0, 0, 0, t.Location())
}

// RefreshWaybar pide a waybar que vuelva a ejecutar el módulo.
func RefreshWaybar(cfg config.Config) {
	_ = exec.Command("pkill", fmt.Sprintf("-RTMIN+%d", cfg.WaybarSignal), "-x", "waybar").Run()
}

// PID devuelve el pid del daemon si está corriendo.
func PID() (int, bool) {
	b, err := os.ReadFile(config.PIDFile())
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 {
		return 0, false
	}
	if err := syscall.Kill(pid, 0); err != nil && !errors.Is(err, syscall.EPERM) {
		return 0, false
	}
	// evita confundir un pid reciclado por otro programa
	exe, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if err == nil && !strings.HasPrefix(filepath.Base(exe), "redus") {
		return 0, false
	}
	return pid, true
}

// Signal envía una señal al daemon; devuelve false si no está corriendo.
func Signal(sig syscall.Signal) bool {
	pid, ok := PID()
	if !ok {
		return false
	}
	return syscall.Kill(pid, sig) == nil
}

func writePID() error {
	if err := os.MkdirAll(filepath.Dir(config.PIDFile()), 0o700); err != nil {
		return err
	}
	return os.WriteFile(config.PIDFile(), []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600)
}
