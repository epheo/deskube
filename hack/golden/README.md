# Golden image

The proof VM disk after `cloud-init clean`, cloned once by thin LVM
snapshot into a DataSource. New desktops clone it the same way.

    oc -n deskube-dev apply -f golden.yaml     # 7 s with cloneType snapshot
    ./up.sh desk-01 31989                      # VM + Service, base port 31989
    ./down.sh desk-01

To change the image: `./rebuild.sh customize.sh` starts the proof VM,
runs the script inside, cleans instance state, stops it, and
re-snapshots.

Measured 2026-09-23: apply to VM Ready 24 s, Sunshine listening 56 s.
The per instance cloud-init only sets hostname, port, and credentials.
This is what the operator renders from `internal/guest`.
