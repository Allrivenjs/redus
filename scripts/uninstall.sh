#!/usr/bin/env bash
# Quita redus del sistema y devuelve waybar al layout anterior.
# No borra cuentas ni configuración (~/.config/redus).
set -euo pipefail

cfg="${XDG_CONFIG_HOME:-$HOME/.config}"
wb="$cfg/waybar"
state="${XDG_STATE_HOME:-$HOME/.local/state}/hyde/staterc"

systemctl --user disable --now redus.service 2>/dev/null || true
rm -f "$cfg/systemd/user/redus.service"
systemctl --user daemon-reload

prev="$(cat "$wb/layouts/.redus-previous" 2>/dev/null || true)"
if [[ -n "$prev" && -f "$prev" && -f "$state" ]]; then
  name="$(basename "$prev" .jsonc)"
  sed -i "s|^WAYBAR_LAYOUT_PATH=.*|WAYBAR_LAYOUT_PATH=$prev|; s|^WAYBAR_LAYOUT_NAME=.*|WAYBAR_LAYOUT_NAME=$name|" "$state"
  cp "$prev" "$wb/config.jsonc"
fi
rm -f "$wb/modules/redus.jsonc" "$wb/layouts/redus.jsonc" "$wb/layouts/.redus-previous"
[[ -f "$wb/user-style.css" ]] && sed -i '/\/\* >>> redus \*\//,/\/\* <<< redus \*\//d' "$wb/user-style.css"
pkill -SIGUSR2 -x waybar || true
rm -f "${XDG_DATA_HOME:-$HOME/.local/share}/applications/redus-open.desktop"
rm -f "$HOME/.local/bin/redus"
echo "redus desinstalado. Tus cuentas siguen en $cfg/redus (bórralo si no las quieres)."
