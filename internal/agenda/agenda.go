// Package agenda genera la vista del día (HTML local) y la abre como ventana
// flotante en Hyprland.
package agenda

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"time"

	"github.com/Allrivenjs/redus/internal/browser"
	"github.com/Allrivenjs/redus/internal/config"
	"github.com/Allrivenjs/redus/internal/store"
)

//go:embed agenda.html.tmpl
var pageTmpl string

// Title es el título de la ventana; sirve para encontrarla en Hyprland.
const Title = "redus · agenda"

const pxPerHour = 64

var palette = []string{"#89b4fa", "#cba6f7", "#a6e3a1", "#fab387", "#94e2d5", "#f5c2e7", "#f9e2af"}

// Block es una reunión ya ubicada en la grilla del día.
type Block struct {
	Title    string
	Account  string
	Color    string
	Link     template.URL // redus:// hacia la videollamada o el evento
	HasJoin  bool
	TimeText string
	Top      int     // px desde el inicio de la grilla
	Height   int     // px
	Left     float64 // % del ancho
	Width    float64 // %
	StartMS  int64
	EndMS    int64
	Short    bool // muy corta para mostrar dos líneas
}

type Hour struct {
	Label string
	Top   int
}

type AllDay struct {
	Title   string
	Account string
	Color   string
}

type Upcoming struct {
	Time    string
	Title   string
	Account string
	Color   string
}

type Legend struct {
	Account string
	Color   string
}

type Page struct {
	Title       string
	DateLabel   string
	Summary     string
	Blocks      []Block
	Hours       []Hour
	GridHeight  int
	GridStartMS int64
	PxPerMin    float64
	AllDay      []AllDay
	Tomorrow    []Upcoming
	TomorrowLbl string
	Legend      []Legend
	MultiAcc    bool
	Synced      string
	Errors      []string
	EventsJSON  template.JS
}

// Build arma los datos de la página para el día de now.
func Build(c store.Cache, now time.Time) Page {
	now = now.Local()
	day := startOfDay(now)
	dayEnd := day.AddDate(0, 0, 1)

	colors := map[string]string{}
	for i, a := range c.Accounts {
		colors[a] = palette[i%len(palette)]
	}
	color := func(acc string) string {
		if v, ok := colors[acc]; ok {
			return v
		}
		return palette[0]
	}

	var timed []store.Event
	p := Page{Title: Title, MultiAcc: len(c.Accounts) > 1}
	seen := map[string]bool{} // misma reunión en dos cuentas: se muestra una vez
	for _, e := range c.Events {
		if !e.Start.Before(dayEnd) || !e.End.After(day) {
			continue
		}
		dup := fmt.Sprintf("%s@%d", e.Title, e.Start.Unix())
		if seen[dup] {
			continue
		}
		seen[dup] = true
		if e.AllDay {
			p.AllDay = append(p.AllDay, AllDay{Title: e.Title, Account: e.Account, Color: color(e.Account)})
			continue
		}
		timed = append(timed, e)
	}
	sort.Slice(timed, func(i, j int) bool { return timed[i].Start.Before(timed[j].Start) })

	// rango visible: 07–19 como mínimo, ampliado para que quepa todo
	startH, endH := 7, 19
	for _, e := range timed {
		s := clamp(e.Start.Local(), day, dayEnd)
		en := clamp(e.End.Local(), day, dayEnd)
		startH = min(startH, s.Hour())
		eh := en.Hour()
		if en.Minute() > 0 || en.Equal(dayEnd) {
			eh++
		}
		if en.Equal(dayEnd) {
			eh = 24
		}
		endH = max(endH, eh)
	}
	endH = min(endH, 24)
	gridStart := day.Add(time.Duration(startH) * time.Hour)
	p.GridStartMS = gridStart.UnixMilli()
	p.PxPerMin = float64(pxPerHour) / 60
	p.GridHeight = (endH - startH) * pxPerHour
	for h := startH; h <= endH; h++ {
		p.Hours = append(p.Hours, Hour{Label: fmt.Sprintf("%02d:00", h%24), Top: (h - startH) * pxPerHour})
	}

	cols := layoutColumns(timed)
	var total time.Duration
	var js []map[string]any
	for i, e := range timed {
		s := clamp(e.Start.Local(), gridStart, dayEnd)
		en := clamp(e.End.Local(), gridStart, dayEnd)
		top := int(s.Sub(gridStart).Minutes() * p.PxPerMin)
		h := max(int(en.Sub(s).Minutes()*p.PxPerMin), 22)
		link := e.JoinURL
		if link == "" {
			link = e.HTMLLink
		}
		// los enlaces pasan por redus para abrirse en el perfil de la cuenta
		via := ""
		if link != "" {
			via = browser.SchemeURL(link, e.Account)
		}
		join := ""
		if e.JoinURL != "" {
			join = browser.SchemeURL(e.JoinURL, e.Account)
		}
		w := 100.0 / float64(cols[i].of)
		b := Block{
			Title:    e.Title,
			Account:  e.Account,
			Color:    color(e.Account),
			Link:     template.URL(via),
			HasJoin:  e.JoinURL != "",
			TimeText: e.Start.Local().Format("15:04") + "–" + e.End.Local().Format("15:04"),
			Top:      top,
			Height:   h,
			Left:     w * float64(cols[i].col),
			Width:    w,
			StartMS:  e.Start.UnixMilli(),
			EndMS:    e.End.UnixMilli(),
			Short:    h < 40,
		}
		p.Blocks = append(p.Blocks, b)
		total += en.Sub(s)
		js = append(js, map[string]any{
			"title": e.Title, "start": b.StartMS, "end": b.EndMS,
			"join": join, "account": e.Account, "time": b.TimeText,
		})
	}
	ej, _ := json.Marshal(js)
	if js == nil {
		ej = []byte("[]")
	}
	p.EventsJSON = template.JS(ej)

	p.DateLabel = fmt.Sprintf("%s %d de %s", weekdays[now.Weekday()], now.Day(), months[now.Month()-1])
	switch n := len(timed); n {
	case 0:
		p.Summary = "Día libre de reuniones"
	default:
		r := "reuniones"
		if n == 1 {
			r = "reunión"
		}
		p.Summary = fmt.Sprintf("%d %s · %s en reuniones · primera %s · última termina %s",
			n, r, hm(total), timed[0].Start.Local().Format("15:04"), lastEnd(timed).Format("15:04"))
	}

	tomorrow := dayEnd
	p.TomorrowLbl = "Mañana"
	if now.Weekday() == time.Friday {
		tomorrow = day.AddDate(0, 0, 3)
		p.TomorrowLbl = "Lunes"
	}
	tEnd := tomorrow.AddDate(0, 0, 1)
	for _, e := range c.Events {
		if e.AllDay || e.Start.Before(tomorrow) || !e.Start.Before(tEnd) {
			continue
		}
		p.Tomorrow = append(p.Tomorrow, Upcoming{Time: e.Start.Local().Format("15:04"), Title: e.Title, Account: e.Account, Color: color(e.Account)})
	}

	for _, a := range c.Accounts {
		p.Legend = append(p.Legend, Legend{Account: a, Color: colors[a]})
	}
	for acc, err := range c.Errors {
		p.Errors = append(p.Errors, acc+": "+err)
	}
	slices.Sort(p.Errors)
	if !c.UpdatedAt.IsZero() {
		p.Synced = c.UpdatedAt.Local().Format("15:04")
	}
	return p
}

type colPos struct{ col, of int }

// layoutColumns reparte en columnas las reuniones que se superponen.
func layoutColumns(evs []store.Event) []colPos {
	res := make([]colPos, len(evs))
	i := 0
	for i < len(evs) {
		// grupo de reuniones encadenadas por superposición
		j := i
		groupEnd := evs[i].End
		for j+1 < len(evs) && evs[j+1].Start.Before(groupEnd) {
			j++
			if evs[j].End.After(groupEnd) {
				groupEnd = evs[j].End
			}
		}
		var colEnds []time.Time
		for k := i; k <= j; k++ {
			placed := false
			for c, end := range colEnds {
				if !evs[k].Start.Before(end) {
					colEnds[c] = evs[k].End
					res[k].col = c
					placed = true
					break
				}
			}
			if !placed {
				res[k].col = len(colEnds)
				colEnds = append(colEnds, evs[k].End)
			}
		}
		for k := i; k <= j; k++ {
			res[k].of = len(colEnds)
		}
		i = j + 1
	}
	return res
}

// Render escribe la página.
func Render(p Page) ([]byte, error) {
	t, err := template.New("agenda").Parse(pageTmpl)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	err = t.Execute(&b, p)
	return b.Bytes(), err
}

// File es la ruta del HTML generado.
func File() string { return filepath.Join(config.CacheDir(), "agenda.html") }

// Write genera el HTML con el caché actual.
func Write(c store.Cache, now time.Time) (string, error) {
	b, err := Render(Build(c, now))
	if err != nil {
		return "", err
	}
	path := File()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return "", err
	}
	return path, os.Rename(tmp, path)
}

var weekdays = [...]string{"Domingo", "Lunes", "Martes", "Miércoles", "Jueves", "Viernes", "Sábado"}
var months = [...]string{"enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"}

func hm(d time.Duration) string {
	m := int(d.Round(time.Minute) / time.Minute)
	if m < 60 {
		return fmt.Sprintf("%d min", m)
	}
	if m%60 == 0 {
		return fmt.Sprintf("%d h", m/60)
	}
	return fmt.Sprintf("%d h %02d", m/60, m%60)
}

func lastEnd(evs []store.Event) time.Time {
	var t time.Time
	for _, e := range evs {
		if e.End.After(t) {
			t = e.End
		}
	}
	return t.Local()
}

func clamp(t, lo, hi time.Time) time.Time {
	if t.Before(lo) {
		return lo
	}
	if t.After(hi) {
		return hi
	}
	return t
}

func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}
