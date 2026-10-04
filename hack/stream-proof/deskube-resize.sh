#!/bin/sh
# Switches the dummy display to the size the Moonlight client asked for.
# Sunshine runs it as a prep command with SUNSHINE_CLIENT_WIDTH and
# SUNSHINE_CLIENT_HEIGHT set. "reset" restores the default afterwards.
# Always exits 0: a failed prep command would abort the whole session.
OUT=${DESKUBE_OUTPUT:-DUMMY0}
DEFAULT=${DESKUBE_DEFAULT_MODE:-1920x1080}
export DISPLAY=${DISPLAY:-:0}

if [ "$1" = reset ]; then
  xrandr --output "$OUT" --mode "$DEFAULT"
  exit 0
fi
W=${1:-$SUNSHINE_CLIENT_WIDTH}
H=${2:-$SUNSHINE_CLIENT_HEIGHT}
case "$W" in ''|*[!0-9]*) echo "deskube-resize: no width"; exit 0;; esac
case "$H" in ''|*[!0-9]*) echo "deskube-resize: no height"; exit 0;; esac
MODE="${W}x${H}"

if ! xrandr | grep -q "^ *$MODE "; then
  # Reduced blanking keeps the pixel clock low; 4K at 60 still exceeds
  # what the dummy driver takes, so fall back to 30 Hz.
  for RATE in 60 30; do
    LINE=$(cvt -r "$W" "$H" "$RATE" | sed -n 's/^Modeline "[^"]*" *//p')
    [ -n "$LINE" ] || continue
    xrandr --delmode "$OUT" "$MODE" 2>/dev/null
    xrandr --rmmode "$MODE" 2>/dev/null
    # shellcheck disable=SC2086
    xrandr --newmode "$MODE" $LINE 2>/dev/null || continue
    xrandr --addmode "$OUT" "$MODE" 2>/dev/null && break
  done
fi
xrandr --output "$OUT" --mode "$MODE" && echo "deskube-resize: $MODE" || echo "deskube-resize: cannot set $MODE"
exit 0
