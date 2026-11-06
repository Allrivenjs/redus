package daemon

import (
	"testing"
	"time"

	"github.com/Allrivenjs/redus/internal/config"
	"github.com/Allrivenjs/redus/internal/store"
)

func TestRemind(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t0 := time.Date(2026, 11, 6, 10, 0, 0, 0, time.Local)
	daily := store.Event{ID: "d1", Account: "a@x.com", Title: "Daily", Start: t0, End: t0.Add(15 * time.Minute)}
	same := daily
	same.Account = "b@y.com" // la misma reunión invitada en otra cuenta
	if err := store.SaveCache(store.Cache{Events: []store.Event{daily, same}}); err != nil {
		t.Fatal(err)
	}

	var got []time.Time
	d := &daemon{cfg: config.Default(), notified: map[string]time.Time{}}
	d.sendFn = func(_ store.Event, now time.Time) { got = append(got, now) }

	steps := []struct {
		at   time.Time
		want int // avisos acumulados
	}{
		{t0.Add(-15 * time.Minute), 0}, // muy pronto
		{t0.Add(-10 * time.Minute), 1}, // aviso de 10 min (una vez, aunque esté en dos cuentas)
		{t0.Add(-9 * time.Minute), 1},  // no repite
		{t0.Add(-1 * time.Minute), 2},  // aviso de 1 min
		{t0.Add(-30 * time.Second), 2}, // no repite
		{t0.Add(5 * time.Minute), 2},   // ya empezó: nada
	}
	for _, s := range steps {
		d.remind(s.at)
		if len(got) != s.want {
			t.Fatalf("a las %s: %d avisos, esperaba %d", s.at.Format("15:04:05"), len(got), s.want)
		}
	}
}

func TestRemindLateStartSendsOnlyClosest(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t0 := time.Date(2026, 11, 6, 10, 0, 0, 0, time.Local)
	e := store.Event{ID: "p", Account: "a", Title: "Planning", Start: t0, End: t0.Add(time.Hour)}
	if err := store.SaveCache(store.Cache{Events: []store.Event{e}}); err != nil {
		t.Fatal(err)
	}
	n := 0
	d := &daemon{cfg: config.Default(), notified: map[string]time.Time{}}
	d.sendFn = func(store.Event, time.Time) { n++ }
	d.remind(t0.Add(-30 * time.Second)) // el PC arranca 30 s antes: pasaron los dos umbrales
	d.remind(t0.Add(-20 * time.Second))
	if n != 1 {
		t.Fatalf("%d avisos, esperaba 1", n)
	}
}

func TestRemindMeetingAlreadyStarted(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t0 := time.Date(2026, 11, 7, 8, 0, 0, 0, time.Local)
	e := store.Event{ID: "r", Account: "a", Title: "Reunión 8:00", Start: t0, End: t0.Add(time.Hour)}
	ended := store.Event{ID: "old", Account: "a", Title: "Ya terminó", Start: t0.Add(-2 * time.Hour), End: t0.Add(-time.Hour)}
	if err := store.SaveCache(store.Cache{Events: []store.Event{e, ended}}); err != nil {
		t.Fatal(err)
	}
	var titles []string
	d := &daemon{cfg: config.Default(), notified: map[string]time.Time{}}
	d.sendFn = func(ev store.Event, _ time.Time) { titles = append(titles, ev.Title) }

	d.remind(t0.Add(2 * time.Minute)) // prende el PC a las 8:02
	d.remind(t0.Add(3 * time.Minute)) // no repite
	if len(titles) != 1 || titles[0] != "Reunión 8:00" {
		t.Fatalf("avisos %v, esperaba solo la reunión en curso", titles)
	}
}

func TestShouldOpenAgenda(t *testing.T) {
	cfg := config.Default()
	mon := func(h, m int) time.Time { return time.Date(2026, 11, 9, h, m, 0, 0, time.Local) } // lunes
	boot := mon(8, 2)
	synced := boot.Add(5 * time.Second)

	cases := []struct {
		name  string
		now   time.Time
		start time.Time
		sync  time.Time
		shown string
		want  bool
	}{
		{"lunes 8:02, ya sincronizó", boot.Add(20 * time.Second), boot, synced, "", true},
		{"recién arrancado", boot.Add(5 * time.Second), boot, synced, "", false},
		{"sin red: espera", boot.Add(time.Minute), boot, time.Time{}, "", false},
		{"sin red: abre con caché a los 2 min", boot.Add(2 * time.Minute), boot, time.Time{}, "", true},
		{"ya se mostró hoy", boot.Add(time.Minute), boot, synced, "2026-11-09", false},
		{"se mostró ayer", boot.Add(time.Minute), boot, synced, "2026-11-06", true},
		{"después de mediodía", mon(13, 0), mon(12, 59), mon(12, 59).Add(time.Second), "", false},
		{"sábado", boot.AddDate(0, 0, 5).Add(time.Minute), boot.AddDate(0, 0, 5), synced.AddDate(0, 0, 5), "", false},
	}
	for _, c := range cases {
		if got := shouldOpenAgenda(cfg, c.now, c.start, c.sync, c.shown); got != c.want {
			t.Errorf("%s: %v, esperaba %v", c.name, got, c.want)
		}
	}
}
