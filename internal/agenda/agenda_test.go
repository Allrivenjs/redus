package agenda

import (
	"strings"
	"testing"
	"time"

	"github.com/Allrivenjs/redus/internal/store"
)

func at(h, m int) time.Time { return time.Date(2026, 11, 6, h, m, 0, 0, time.Local) }

func ev(id string, s, e time.Time, acc string) store.Event {
	return store.Event{ID: id, Title: id, Start: s, End: e, Account: acc, JoinURL: "https://meet.google.com/" + id}
}

func TestBuildLayout(t *testing.T) {
	c := store.Cache{
		Accounts: []string{"a@x", "b@y"},
		Events: []store.Event{
			ev("daily", at(9, 0), at(9, 15), "a@x"),
			ev("1a1", at(10, 0), at(11, 0), "a@x"),
			ev("cruce", at(10, 30), at(11, 30), "b@y"),                                 // se superpone con 1a1
			ev("tarde", at(21, 0), at(22, 0), "b@y"),                                   // amplía el rango
			{ID: "x", Title: "daily", Start: at(9, 0), End: at(9, 15), Account: "b@y"}, // duplicada en otra cuenta
			{ID: "fer", Title: "Feriado", AllDay: true, Start: at(0, 0), End: at(0, 0).AddDate(0, 0, 1), Account: "a@x"},
			ev("ayer", at(9, 0).AddDate(0, 0, -1), at(10, 0).AddDate(0, 0, -1), "a@x"),
		},
	}
	p := Build(c, at(8, 0))
	if len(p.Blocks) != 4 {
		t.Fatalf("bloques %d, esperaba 4 (sin duplicada, sin día completo, sin ayer)", len(p.Blocks))
	}
	if len(p.AllDay) != 1 {
		t.Fatalf("día completo: %d", len(p.AllDay))
	}
	b := map[string]Block{}
	for _, x := range p.Blocks {
		b[x.Title] = x
	}
	if b["1a1"].Width != 50 || b["cruce"].Width != 50 || b["cruce"].Left != 50 {
		t.Fatalf("superpuestas mal repartidas: %+v %+v", b["1a1"], b["cruce"])
	}
	if b["daily"].Width != 100 {
		t.Fatalf("daily debería ocupar todo el ancho: %+v", b["daily"])
	}
	if p.Hours[0].Label != "07:00" || p.Hours[len(p.Hours)-1].Label != "22:00" {
		t.Fatalf("rango %s–%s", p.Hours[0].Label, p.Hours[len(p.Hours)-1].Label)
	}
	if b["daily"].Top != 2*pxPerHour { // 09:00 con la grilla desde 07:00
		t.Fatalf("top daily %d", b["daily"].Top)
	}
	if !strings.HasPrefix(p.Summary, "4 reuniones") {
		t.Fatalf("resumen %q", p.Summary)
	}
	if p.DateLabel != "Viernes 6 de noviembre" || p.TomorrowLbl != "Lunes" {
		t.Fatalf("fecha %q / %q", p.DateLabel, p.TomorrowLbl)
	}
	html, err := Render(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(html), "<title>redus · agenda</title>") {
		t.Fatal("falta el título de la ventana")
	}
}

func TestRenderEscapes(t *testing.T) {
	c := store.Cache{Accounts: []string{"a"}, Events: []store.Event{
		{ID: "1", Title: `<script>alert(1)</script>`, Start: at(10, 0), End: at(11, 0), Account: "a", JoinURL: "javascript:alert(1)"},
	}}
	html, err := Render(Build(c, at(8, 0)))
	if err != nil {
		t.Fatal(err)
	}
	s := string(html)
	if strings.Contains(s, "<script>alert(1)</script>") || strings.Contains(s, `href="javascript:`) {
		t.Fatal("contenido del evento sin escapar")
	}
}
