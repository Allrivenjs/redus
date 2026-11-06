// Package calls detecta si hay una llamada en curso mirando qué programas
// capturan el micrófono en PipeWire/PulseAudio.
//
// Se parsea la salida en texto de pactl: `pactl -f json` (pactl 17) emite JSON
// inválido cuando hay streams, por el campo Format sin escapar.
package calls

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Stream es un programa que está capturando audio de una fuente real.
type Stream struct {
	App    string // application.name
	Binary string // application.process.binary
	Media  string // media.name
}

// Label es el nombre que se le muestra al usuario.
func (s Stream) Label() string {
	if s.App != "" {
		return s.App
	}
	return s.Binary
}

// Detector decide qué capturas cuentan como llamada.
type Detector struct {
	Ignore []string
}

// Active devuelve las capturas de micrófono que cuentan como llamada.
// Excluye monitores (captura del audio que sale, como cava), streams
// pausados (corked) y los programas en la lista de ignorados.
func (d Detector) Active(ctx context.Context) ([]Stream, error) {
	monitors, err := monitorSources(ctx)
	if err != nil {
		return nil, err
	}
	out, err := pactl(ctx, "list", "source-outputs")
	if err != nil {
		return nil, err
	}
	var res []Stream
	for _, o := range parseSourceOutputs(out) {
		if o.corked || monitors[o.source] {
			continue
		}
		if d.ignored(o.Stream) {
			continue
		}
		res = append(res, o.Stream)
	}
	return res, nil
}

func (d Detector) ignored(s Stream) bool {
	app := strings.ToLower(s.App)
	bin := strings.ToLower(s.Binary)
	for _, ig := range d.Ignore {
		ig = strings.ToLower(strings.TrimSpace(ig))
		if ig == "" {
			continue
		}
		if strings.Contains(app, ig) || strings.Contains(bin, ig) {
			return true
		}
	}
	return false
}

type sourceOutput struct {
	Stream
	source int
	corked bool
}

func parseSourceOutputs(text string) []sourceOutput {
	var res []sourceOutput
	var cur *sourceOutput
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "Source Output #") {
			res = append(res, sourceOutput{source: -1})
			cur = &res[len(res)-1]
			continue
		}
		if cur == nil {
			continue
		}
		t := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(t, "Source: "):
			if n, err := strconv.Atoi(strings.TrimPrefix(t, "Source: ")); err == nil {
				cur.source = n
			}
		case strings.HasPrefix(t, "Corked: "):
			cur.corked = strings.TrimPrefix(t, "Corked: ") == "yes"
		case strings.HasPrefix(t, "application.name = "):
			cur.App = unquote(t, "application.name = ")
		case strings.HasPrefix(t, "application.process.binary = "):
			cur.Binary = unquote(t, "application.process.binary = ")
		case strings.HasPrefix(t, "media.name = "):
			cur.Media = unquote(t, "media.name = ")
		}
	}
	return res
}

func unquote(line, prefix string) string {
	v := strings.TrimPrefix(line, prefix)
	if s, err := strconv.Unquote(v); err == nil {
		return s
	}
	return strings.Trim(v, `"`)
}

// monitorSources devuelve los índices de las fuentes que son monitores de salida.
func monitorSources(ctx context.Context) (map[int]bool, error) {
	out, err := pactl(ctx, "list", "short", "sources")
	if err != nil {
		return nil, err
	}
	return parseMonitors(out), nil
}

func parseMonitors(text string) map[int]bool {
	m := map[int]bool{}
	for _, line := range strings.Split(text, "\n") {
		f := strings.Split(line, "\t")
		if len(f) < 2 {
			continue
		}
		n, err := strconv.Atoi(f[0])
		if err != nil {
			continue
		}
		if strings.HasSuffix(f[1], ".monitor") {
			m[n] = true
		}
	}
	return m
}

func pactl(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "pactl", args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	b, err := cmd.Output()
	return string(b), err
}
