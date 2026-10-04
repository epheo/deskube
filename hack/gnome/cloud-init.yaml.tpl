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
      global_prep_cmd = [{"do":"sh -c \"/usr/local/bin/deskube-resize || true\"","undo":"sh -c \"/usr/local/bin/deskube-resize reset || true\""}]
      min_log_level = info
      file_state = /home/desk/.config/sunshine/sunshine_state.json
      credentials_file = /home/desk/.config/sunshine/sunshine_state.json
      log_path = /home/desk/.config/sunshine/sunshine.log
runcmd:
  - chown -R desk:desk /home/desk/.config
  - sudo -u desk /usr/bin/sunshine /home/desk/.config/sunshine/sunshine.conf --creds admin '__SUNSHINE_PASS__'
  - systemctl --user -M desk@ restart sunshine.service || true
  - touch /var/lib/cloud/deskube-provisioned
