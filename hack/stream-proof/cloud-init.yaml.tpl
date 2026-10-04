#cloud-config
hostname: desk-proof
users:
  - name: desk
    groups: [wheel, input]
    shell: /bin/bash
    sudo: ALL=(ALL) NOPASSWD:ALL
    lock_passwd: true
    ssh_authorized_keys:
      - __SSH_PUBKEY__
packages:
  - xorg-x11-server-Xorg
  - xorg-x11-drv-dummy
  - xorg-x11-drv-libinput
  - xfce4-session
  - xfce4-panel
  - xfwm4
  - xfdesktop
  - xfce4-terminal
  - xterm
  - dbus-x11
  - pipewire
  - pipewire-pulseaudio
  - wireplumber
  - xrandr
  - cvt
write_files:
  # Sync ranges and Virtual are wide on purpose: deskube-resize adds any
  # mode a client asks for at runtime, up to 4K.
  - path: /etc/X11/xorg-dummy.conf
    content: |
      Section "ServerFlags"
        Option "AutoAddGPU" "false"
      EndSection
      Section "Device"
        Identifier "dummy"
        Driver "dummy"
        VideoRam 256000
      EndSection
      Section "Monitor"
        Identifier "monitor"
        HorizSync 5.0-300.0
        VertRefresh 5.0-120.0
        Option "ReducedBlanking" "true"
        Modeline "1280x720"  63.75 1280 1328 1360 1440 720 723 728 741 +hsync -vsync
        Modeline "1366x768"  72.00 1366 1414 1446 1526 768 771 776 790 +hsync -vsync
        Modeline "1600x900"  97.75 1600 1648 1680 1760 900 903 908 926 +hsync -vsync
        Modeline "1920x1080" 138.50 1920 1968 2000 2080 1080 1083 1088 1111 +hsync -vsync
        Modeline "1920x1200" 154.00 1920 1968 2000 2080 1200 1203 1209 1235 +hsync -vsync
        Modeline "2560x1440" 241.50 2560 2608 2640 2720 1440 1443 1448 1481 +hsync -vsync
        Modeline "2560x1600" 268.50 2560 2608 2640 2720 1600 1603 1609 1646 +hsync -vsync
      EndSection
      Section "Screen"
        Identifier "screen"
        Device "dummy"
        Monitor "monitor"
        DefaultDepth 24
        SubSection "Display"
          Depth 24
          Modes "1920x1080" "2560x1440" "1920x1200" "1600x900" "1366x768" "1280x720"
          Virtual 3840 2160
        EndSubSection
      EndSection
      Section "ServerLayout"
        Identifier "layout"
        Screen "screen"
      EndSection
  - path: /usr/local/bin/deskube-resize
    permissions: "0755"
    content: |
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
  - path: /etc/systemd/system/xorg-dummy.service
    content: |
      [Unit]
      Description=Headless Xorg on the dummy driver
      After=systemd-udevd.service
      [Service]
      ExecStart=/usr/bin/Xorg :0 -config /etc/X11/xorg-dummy.conf -noreset -ac -nolisten tcp vt7
      Restart=always
      [Install]
      WantedBy=multi-user.target
  # The package rule uses uaccess, which only covers a seated login.
  # A lingering user has no seat, so grant the input group directly.
  - path: /etc/udev/rules.d/70-deskube-uinput.rules
    content: |
      KERNEL=="uinput", SUBSYSTEM=="misc", GROUP="input", MODE="0660", OPTIONS+="static_node=uinput"
  - path: /etc/modules-load.d/uinput.conf
    content: uinput
  - path: /home/desk/.config/systemd/user/xfce.service
    owner: desk:desk
    defer: true
    content: |
      [Unit]
      Description=XFCE session on display :0
      [Service]
      Environment=DISPLAY=:0
      ExecStartPre=/bin/sh -c 'until [ -S /tmp/.X11-unix/X0 ]; do sleep 1; done'
      ExecStart=/usr/bin/xfce4-session
      Restart=always
      RestartSec=3
      [Install]
      WantedBy=default.target
  - path: /home/desk/.config/systemd/user/sunshine.service
    owner: desk:desk
    defer: true
    content: |
      [Unit]
      Description=Sunshine streaming host
      After=xfce.service pipewire-pulse.service wireplumber.service
      Wants=xfce.service pipewire-pulse.service wireplumber.service
      [Service]
      Environment=DISPLAY=:0
      ExecStartPre=/bin/sh -c 'until [ -S /tmp/.X11-unix/X0 ]; do sleep 1; done'
      ExecStart=/usr/bin/sunshine /home/desk/.config/sunshine/sunshine.conf
      Restart=on-failure
      RestartSec=3
      [Install]
      WantedBy=default.target
  # Sunshine loads a null sink and sets it default in one go, which races
  # PipeWire and fails. A sink that already exists at boot is found by name.
  - path: /home/desk/.config/pipewire/pipewire.conf.d/10-sunshine-sink.conf
    owner: desk:desk
    defer: true
    content: |
      context.objects = [
        { factory = adapter
          args = {
            factory.name     = support.null-audio-sink
            node.name        = "sink-sunshine-stereo"
            node.description = "Sunshine"
            media.class      = Audio/Sink
            object.linger    = true
            audio.position   = [ FL FR ]
          }
        }
      ]
  # port 31989 keeps every derived port inside the NodePort range,
  # so the Service can map node ports 1:1 and Moonlight derives the rest.
  - path: /home/desk/.config/sunshine/sunshine.conf
    owner: desk:desk
    defer: true
    content: |
      sunshine_name = desk-proof
      port = 31989
      address_family = ipv4
      origin_web_ui_allowed = wan
      capture = x11
      encoder = software
      sw_preset = superfast
      sw_tune = zerolatency
      global_prep_cmd = [{"do":"sh -c \"/usr/local/bin/deskube-resize || true\"","undo":"sh -c \"/usr/local/bin/deskube-resize reset || true\""}]
      min_log_level = info
      file_state = /home/desk/.config/sunshine/sunshine_state.json
      credentials_file = /home/desk/.config/sunshine/sunshine_state.json
      log_path = /home/desk/.config/sunshine/sunshine.log
runcmd:
  - dnf install -y https://github.com/LizardByte/Sunshine/releases/download/v__SUNSHINE_VERSION__/Sunshine-__SUNSHINE_VERSION__-1.fc44.x86_64.rpm
  - modprobe uinput
  - udevadm control --reload
  - udevadm trigger --subsystem-match=misc
  - chown -R desk:desk /home/desk/.config
  - sudo -u desk /usr/bin/sunshine /home/desk/.config/sunshine/sunshine.conf --creds admin '__SUNSHINE_PASS__'
  - systemctl daemon-reload
  - systemctl enable --now xorg-dummy.service
  - mkdir -p /home/desk/.config/systemd/user/default.target.wants
  - ln -sf ../xfce.service /home/desk/.config/systemd/user/default.target.wants/xfce.service
  - ln -sf ../sunshine.service /home/desk/.config/systemd/user/default.target.wants/sunshine.service
  - chown -R desk:desk /home/desk/.config
  - loginctl enable-linger desk
  # An early SSH login can start the user manager before the symlinks exist.
  - systemctl --user -M desk@ daemon-reload
  - systemctl --user -M desk@ start xfce.service sunshine.service
  - touch /var/lib/cloud/deskube-provisioned
