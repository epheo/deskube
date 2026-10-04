#cloud-config
hostname: __NAME__
users:
  - name: desk
    ssh_authorized_keys:
      - __SSH_PUBKEY__
write_files:
  - path: /home/desk/.config/sunshine/sunshine.conf
    owner: desk:desk
    defer: true
    content: |
      sunshine_name = __NAME__
      port = __PORT__
      address_family = ipv4
      origin_web_ui_allowed = wan
      capture = kms
      encoder = software
      sw_preset = superfast
      sw_tune = zerolatency
      global_prep_cmd = [{"do":"sh -c \"/usr/bin/deskube-resize || true\"","undo":"sh -c \"/usr/bin/deskube-resize reset || true\""}]
      min_log_level = info
      file_state = /home/desk/.config/sunshine/sunshine_state.json
      credentials_file = /home/desk/.config/sunshine/sunshine_state.json
      log_path = /home/desk/.config/sunshine/sunshine.log
runcmd:
  # The image may ship defaults (capture, encoder) for keys the instance
  # left unset; Sunshine has no layered config of its own.
  - |
    conf=/home/desk/.config/sunshine/sunshine.conf
    [ -f /etc/deskube/sunshine.defaults ] && grep -E '^[a-z_]+ *=' /etc/deskube/sunshine.defaults | while read -r key rest; do
      grep -q "^$key *=" "$conf" || echo "$key $rest" >> "$conf"
    done
    true
  - chown -R desk:desk /home/desk/.config
  - sudo -u desk /usr/bin/sunshine /home/desk/.config/sunshine/sunshine.conf --creds admin '__SUNSHINE_PASS__'
  # systemctl --user -M fails from cloud-init on bootc; runuser with the
  # user's runtime dir reaches the same user manager everywhere.
  - runuser -u desk -- env XDG_RUNTIME_DIR=/run/user/$(id -u desk) DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/$(id -u desk)/bus systemctl --user restart sunshine.service
  - touch /var/lib/cloud/deskube-provisioned
