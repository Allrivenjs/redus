#!/usr/bin/env bash
# Instala redus: binario, servicio de systemd y módulo de waybar (HyDE).
# Es idempotente: se puede correr de nuevo tras cada cambio.
set -euo pipefail

repo="$(cd "$(dirname "$0")/.." && pwd)"
cfg="${XDG_CONFIG_HOME:-$HOME/.config}"
bin="$HOME/.local/bin"
wb="$cfg/waybar"
state="${XDG_STATE_HOME:-$HOME/.local/state}/hyde/staterc"

echo "==> Compilando"
mkdir -p "$bin"
(cd "$repo" && go build -o "$bin/redus" ./cmd/redus)

echo "==> Servicio de systemd"
mkdir -p "$cfg/systemd/user"
cp "$repo/contrib/systemd/redus.service" "$cfg/systemd/user/redus.service"
systemctl --user daemon-reload
systemctl --user enable redus.service >/dev/null
systemctl --user restart redus.service

echo "==> Protocolo redus:// (enlaces de la agenda)"
apps="${XDG_DATA_HOME:-$HOME/.local/share}/applications"
mkdir -p "$apps"
sed "s|^Exec=redus |Exec=$bin/redus |" "$repo/contrib/desktop/redus-open.desktop" >"$apps/redus-open.desktop"
xdg-mime default redus-open.desktop x-scheme-handler/redus
update-desktop-database "$apps" 2>/dev/null || true

echo "==> Módulo de waybar"
mkdir -p "$wb/modules" "$wb/layouts"
cp "$repo/contrib/waybar/redus.jsonc" "$wb/modules/redus.jsonc"

# HyDE vuelve a copiar el layout guardado sobre config.jsonc al arrancar, así
# que el módulo se agrega a un layout propio y se apunta el estado de HyDE a él.
layout="$wb/layouts/redus.jsonc"
current="$(grep -m1 '^WAYBAR_LAYOUT_PATH=' "$state" 2>/dev/null | cut -d= -f2- || true)"
if [[ "$current" != "$layout" ]]; then
  src="${current:-$wb/config.jsonc}"
  [[ -f "$src" ]] || { echo "No encuentro el layout actual ($src)"; exit 1; }
  echo "    layout base: $src"
  if grep -q '"custom/redus"' "$src"; then
    cp "$src" "$layout"
  elif grep -q '"clock"' "$src"; then
    # lo pone justo después del reloj (solo la primera aparición)
    sed '0,/"clock"/s//"clock", "custom\/redus"/' "$src" >"$layout"
  else
    echo "El layout no tiene \"clock\"; agrega \"custom/redus\" a mano en $layout"
    cp "$src" "$layout"
  fi
  if [[ -f "$state" ]]; then
    cp "$state" "$state.redus-bak"
    sed -i "s|^WAYBAR_LAYOUT_PATH=.*|WAYBAR_LAYOUT_PATH=$layout|; s|^WAYBAR_LAYOUT_NAME=.*|WAYBAR_LAYOUT_NAME=redus|" "$state"
    grep -q '^WAYBAR_LAYOUT_PATH=' "$state" || echo "WAYBAR_LAYOUT_PATH=$layout" >>"$state"
    grep -q '^WAYBAR_LAYOUT_NAME=' "$state" || echo "WAYBAR_LAYOUT_NAME=redus" >>"$state"
    echo "$current" >"$wb/layouts/.redus-previous"
  fi
fi
cp "$wb/config.jsonc" "$wb/config.jsonc.redus-bak" 2>/dev/null || true
cp "$layout" "$wb/config.jsonc"

# estilos: bloque delimitado en user-style.css (el archivo que HyDE deja al usuario)
css="$wb/user-style.css"
touch "$css"
sed -i '/\/\* >>> redus \*\//,/\/\* <<< redus \*\//d' "$css"
cat "$repo/contrib/waybar/redus.css" >>"$css"

echo "==> Recargando waybar"
pkill -SIGUSR2 -x waybar || echo "    waybar no está corriendo"

echo
"$bin/redus" status
