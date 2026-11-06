package agenda

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Open muestra la agenda como ventana flotante. Usa el navegador en modo app
// (sin barras); si ya hay una ventana de agenda abierta, la reemplaza. Fuera
// de Hyprland, o sin ese navegador, cae a xdg-open.
func Open(path, browser string, width, height int) error {
	env := sessionEnv()
	url := "file://" + path

	if browser != "" {
		if _, err := exec.LookPath(browser); err == nil && env.has("HYPRLAND_INSTANCE_SIGNATURE") {
			for _, addr := range findWindows(env) {
				hyprDispatch(env, fmt.Sprintf(`hl.dsp.window.close({ window = "address:%s" })`, addr),
					"closewindow address:"+addr)
			}
			cmd := exec.Command(browser, "--app="+url)
			cmd.Env = env.list()
			if err := cmd.Start(); err != nil {
				return err
			}
			go cmd.Wait()
			// la ventana la crea el proceso del navegador que ya estaba
			// abierto, así que se la busca por título y se ajusta después
			for i := 0; i < 40; i++ {
				time.Sleep(250 * time.Millisecond)
				if w := findWindows(env); len(w) > 0 {
					a := w[0]
					hyprDispatch(env, fmt.Sprintf(`hl.dsp.window.float({ action = "enable", window = "address:%s" })`, a),
						"setfloating address:"+a)
					hyprDispatch(env, fmt.Sprintf(`hl.dsp.window.resize({ x = %d, y = %d, window = "address:%s" })`, width, height, a),
						fmt.Sprintf("resizewindowpixel exact %d %d,address:%s", width, height, a))
					hyprDispatch(env, "", "centerwindow")
					return nil
				}
			}
			return nil // abrió, aunque no se pudo ajustar
		}
	}
	cmd := exec.Command("xdg-open", url)
	cmd.Env = env.list()
	return cmd.Start()
}

type hyprClient struct {
	Address string `json:"address"`
	Title   string `json:"title"`
}

func findWindows(env envMap) []string {
	cmd := exec.Command("hyprctl", "clients", "-j")
	cmd.Env = env.list()
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var cs []hyprClient
	if json.Unmarshal(out, &cs) != nil {
		return nil
	}
	var res []string
	for _, c := range cs {
		if c.Title == Title {
			res = append(res, c.Address)
		}
	}
	return res
}

// hyprDispatch prueba la sintaxis Lua (Hyprland ≥ 0.55) y, si falla, la clásica.
func hyprDispatch(env envMap, lua, classic string) {
	if lua != "" {
		cmd := exec.Command("hyprctl", "dispatch", lua)
		cmd.Env = env.list()
		out, err := cmd.CombinedOutput()
		if err == nil && strings.TrimSpace(string(out)) == "ok" {
			return
		}
	}
	if classic == "" {
		return
	}
	args := append([]string{"dispatch"}, strings.SplitN(classic, " ", 2)...)
	cmd := exec.Command("hyprctl", args...)
	cmd.Env = env.list()
	_ = cmd.Run()
}

type envMap map[string]string

func (e envMap) has(k string) bool { return e[k] != "" }

func (e envMap) list() []string {
	res := make([]string, 0, len(e))
	for k, v := range e {
		res = append(res, k+"="+v)
	}
	return res
}

// sessionEnv toma el entorno del proceso y le suma el de la sesión de
// systemd: al arrancar el PC el daemon puede iniciar antes de que Hyprland
// exporte WAYLAND_DISPLAY y HYPRLAND_INSTANCE_SIGNATURE.
func sessionEnv() envMap {
	env := envMap{}
	for _, kv := range os.Environ() {
		if k, v, ok := strings.Cut(kv, "="); ok {
			env[k] = v
		}
	}
	out, err := exec.Command("systemctl", "--user", "show-environment").Output()
	if err != nil {
		return env
	}
	for _, line := range strings.Split(string(out), "\n") {
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch k {
		case "WAYLAND_DISPLAY", "DISPLAY", "HYPRLAND_INSTANCE_SIGNATURE", "XDG_CURRENT_DESKTOP",
			"XDG_SESSION_TYPE", "DBUS_SESSION_BUS_ADDRESS":
			env[k] = v
		}
	}
	return env
}
