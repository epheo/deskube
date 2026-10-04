#!/bin/sh
# Switches the dummy display to the size the Moonlight client asked for.
# Sunshine runs it as a prep command with SUNSHINE_CLIENT_WIDTH and
# SUNSHINE_CLIENT_HEIGHT set. "reset" restores the default afterwards.
# Always exits 0: a failed prep command would abort the whole session.
OUT=${DESKUBE_OUTPUT:-DUMMY0}
DEFAULT=${DESKUBE_DEFAULT_MODE:-1920x1080}
export DISPLAY=${DISPLAY:-:0}
# Sunshine does not log what prep commands print, so keep our own trail.
LOG=${DESKUBE_RESIZE_LOG:-$HOME/.cache/deskube-resize.log}
mkdir -p "$(dirname "$LOG")" 2>/dev/null
log() { echo "$(date +%FT%T) $*" | tee -a "$LOG"; }

if [ "$1" = reset ]; then
  xrandr --output "$OUT" --mode "$DEFAULT" && log "reset to $DEFAULT" || log "reset failed"
  exit 0
fi
W=${1:-$SUNSHINE_CLIENT_WIDTH}
H=${2:-$SUNSHINE_CLIENT_HEIGHT}
case "$W" in ''|*[!0-9]*) log "no width in request (W='$W' H='$H')"; exit 0;; esac
case "$H" in ''|*[!0-9]*) log "no height in request (W='$W' H='$H')"; exit 0;; esac
MODE="${W}x${H}"

if ! xrandr | grep -q "^ *$MODE "; then
  # Reduced blanking keeps the pixel clock low; 4K at 60 still exceeds
  # what the dummy driver takes, so fall back to 30 Hz.
  for RATE in 60 30; do
    LINE=$(cvt -r "$W" "$H" "$RATE" | sed -n 's/^Modeline "[^"]*" *//p')
    [ -n "$LINE" ] || { log "cvt has no $MODE at $RATE Hz"; continue; }
    xrandr --delmode "$OUT" "$MODE" 2>/dev/null
    xrandr --rmmode "$MODE" 2>/dev/null
    # shellcheck disable=SC2086
    xrandr --newmode "$MODE" $LINE 2>/dev/null || continue
    xrandr --addmode "$OUT" "$MODE" 2>/dev/null && break
  done
fi
xrandr --output "$OUT" --mode "$MODE" && log "set $MODE (client asked ${SUNSHINE_CLIENT_WIDTH}x${SUNSHINE_CLIENT_HEIGHT} at ${SUNSHINE_CLIENT_FPS} fps)" || log "cannot set $MODE"
exit 0
