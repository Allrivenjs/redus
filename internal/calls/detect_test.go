package calls

import "testing"

const sample = `Source Output #283
	Driver: PipeWire
	Owner Module: n/a
	Client: 282
	Source: 69
	Format: pcm, format.sample_format = "\"s16le\""  format.rate = "44100"
	Corked: no
	Mute: no
	Properties:
		application.name = "Google Chrome"
		media.name = "WebRTC"
		application.process.binary = "chrome"

Source Output #290
	Source: 70
	Corked: no
	Properties:
		application.name = "cava"
		application.process.binary = "cava"

Source Output #291
	Source: 69
	Corked: no
	Properties:
		application.name = "WEBRTC VoiceEngine"
		application.process.binary = "Discord"

Source Output #292
	Source: 69
	Corked: yes
	Properties:
		application.name = "Zoom"
		application.process.binary = "zoom"
`

const sources = "69\talsa_input.usb-mic\tPipeWire\ts16le 2ch 48000Hz\tRUNNING\n" +
	"70\talsa_output.pci.analog-stereo.monitor\tPipeWire\ts32le 2ch 48000Hz\tIDLE\n"

func TestParseAndFilter(t *testing.T) {
	outs := parseSourceOutputs(sample)
	if len(outs) != 4 {
		t.Fatalf("esperaba 4 streams, hubo %d", len(outs))
	}
	if outs[0].App != "Google Chrome" || outs[0].Binary != "chrome" || outs[0].source != 69 {
		t.Fatalf("stream 0 mal parseado: %+v", outs[0])
	}
	if !outs[3].corked {
		t.Fatal("stream 3 debería estar corked")
	}
	mon := parseMonitors(sources)
	if !mon[70] || mon[69] {
		t.Fatalf("monitores mal detectados: %v", mon)
	}

	d := Detector{Ignore: []string{"discord", "cava"}}
	var active []Stream
	for _, o := range outs {
		if o.corked || mon[o.source] || d.ignored(o.Stream) {
			continue
		}
		active = append(active, o.Stream)
	}
	if len(active) != 1 || active[0].App != "Google Chrome" {
		t.Fatalf("solo Chrome debería contar como llamada, quedó: %+v", active)
	}
}
