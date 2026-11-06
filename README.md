# redus

Avisos de reuniones de varias cuentas de Google en el escritorio (Hyprland +
dunst + waybar), con no molestar automático mientras estás en una llamada.

- **Avisos**: notificación 10 min y 1 min antes de cada reunión, con botón
  *Unirse* que abre el Meet/Zoom/Teams.
- **Barra**: módulo `custom/redus` junto al reloj con la próxima reunión y
  cuenta regresiva; al pasar el mouse, la agenda de hoy y mañana.
- **Llamadas**: si un programa usa el micrófono (salvo Discord, cava, etc.)
  pausa dunst; al colgar lo reanuda y aparece lo que quedó en cola.

## Instalar

```sh
make install     # compila, instala el servicio y el módulo de waybar
redus status     # diagnóstico
```

Requiere Go, `pactl` (PipeWire), `dunstctl`, `notify-send`, `rofi`,
`xdg-open`. El instalador:

1. Deja el binario en `~/.local/bin/redus`.
2. Instala y arranca `redus.service` (systemd de usuario).
3. Crea `~/.config/waybar/layouts/redus.jsonc` (tu layout actual con
   `custom/redus` después del reloj) y apunta HyDE a ese layout. HyDE copia el
   layout sobre `config.jsonc` al arrancar, así que editar `config.jsonc`
   directo no dura.
4. Agrega los estilos en un bloque marcado de `~/.config/waybar/user-style.css`.

`make uninstall` deshace todo y devuelve el layout anterior.

## Conectar cuentas de Google

Se hace una vez. Es gratis.

1. Entra a <https://console.cloud.google.com> y crea un proyecto (p. ej. `redus`).
2. **APIs y servicios → Biblioteca**: habilita **Google Calendar API**.
3. **Google Auth Platform → Público** (pantalla de consentimiento): tipo
   **Externo**, nombre `redus`, tu correo de soporte.
4. En **Público**, pulsa **Publicar app** para pasarla a *En producción*.
   En modo *Prueba* los permisos vencen a los 7 días. Google mostrará "app no
   verificada" al conectar: es tu propia app, entra por *Configuración avanzada
   → Ir a redus*.
5. **Clientes → Crear cliente → App de escritorio**. Descarga el JSON y
   guárdalo como `~/.config/redus/credentials.json`.
6. Conecta cada cuenta (abre el navegador):

   ```sh
   redus auth add tu.correo@gmail.com
   redus auth add otra@empresa.com
   redus auth list
   ```

Solo se pide lectura de calendario (`calendar.readonly`). Los tokens quedan en
`~/.config/redus/tokens/` con permisos 600.

**Cuentas de Google Workspace** (dominio de empresa): si el administrador
bloquea apps externas, la autorización falla con `admin_policy_enforced` y hay
que pedir que permitan el cliente.

## Uso

| Acción en la barra | Qué hace |
|---|---|
| Pasar el mouse | Agenda de hoy y mañana, estado de llamada y errores |
| Clic izquierdo | Menú rofi con las reuniones; Enter abre la videollamada |
| Clic derecho | No molestar manual (siempre en pausa) ↔ automático |
| Clic central | Sincronizar ya |

| Estado | Barra |
|---|---|
| Sin reuniones | `󰃰` atenuado |
| Hoy, en más de 1 h | `󰃰 14:30` |
| En menos de 1 h | `󰃰 Planning · 40m` |
| En menos de 15 min | amarillo |
| En menos de 2 min | rojo parpadeando |
| Reunión en curso | `󰕧 Daily · en curso` |
| En llamada | `󰍬 En llamada · 󰂛` |

CLI:

```
redus status                  diagnóstico
redus sync                    sincronizar ahora
redus dnd [toggle|on|auto]    no molestar manual
redus menu                    menú de reuniones
redus auth add|list|remove    cuentas
```

## Configuración

Opcional, en `~/.config/redus/config.json` (los campos que faltan usan el
valor por defecto):

```json
{
  "remind_minutes": [10, 1],
  "sync_interval": "1m",
  "call_poll_interval": "3s",
  "lookahead_days": 2,
  "ignore_apps": ["discord", "vesktop", "cava", "pavucontrol", "easyeffects", "obs"],
  "waybar_signal": 11,
  "countdown_minutes": 60,
  "urgent_minutes": 2
}
```

`ignore_apps` se compara (sin mayúsculas, por contenido) con el nombre de la
aplicación y el binario que captura el micrófono. Para ver qué detecta:
`redus status` mientras estás en la llamada.

Tras cambiarla: `systemctl --user restart redus`.

## Cómo funciona

- **Llamada**: cada 3 s lee `pactl list source-outputs` (texto; el `-f json`
  de pactl 17 sale inválido) y descarta monitores, streams en pausa y
  programas ignorados.
- **No molestar**: `dunstctl set-paused`. redus solo quita una pausa que puso
  él; si pausaste dunst por otro medio, lo respeta.
- **Calendario**: cada 1 min consulta el calendario principal de cada cuenta y
  guarda el resultado en `~/.cache/redus/events.json`. La barra y el menú leen
  ese caché, no llaman a Google. Omite eventos cancelados, rechazados, de día
  completo, fuera de oficina y de concentración.
- **Avisos**: si al arrancar ya pasaron varios umbrales, manda solo el más
  cercano. Si una reunión ya empezó y no se avisó (el PC estaba apagado),
  avisa "En curso: empezó hace N min" mientras no termine. Una reunión
  invitada en dos cuentas avisa una vez.

Logs: `journalctl --user -u redus -f`.

## Pendiente

- Gmail: avisos de invitaciones nuevas, cambios y cancelaciones.
