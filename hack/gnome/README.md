# GNOME desktop image

`images/desktop-gnome/` is a bootc image: Fedora rawhide, GNOME on
Wayland, Sunshine with KMS capture, cloud-init for per desktop settings.

    podman build -t registry.desku.be/deskube/desktop-gnome:rawhide images/desktop-gnome
    podman push registry.desku.be/deskube/desktop-gnome:rawhide

`image.sh` makes a DataSource out of it with no privileged build: a
builder VM boots Fedora CoreOS, runs `bootc switch` to the image, powers
off, and its disk is snapshotted. The CoreOS base is a DataVolume
imported from `quay.io/fedora/fedora-coreos-kubevirt:stable`.

    ./image.sh desktop-gnome-rawhide registry.desku.be/deskube/desktop-gnome:rawhide
    ./up.sh desk-gnome 31989          # a desktop from it, base port 31989
    ./down.sh desk-gnome

The registry pull secret lands in `/etc/ostree/auth.json` of the image,
which is what `bootc upgrade` needs later inside the desktops.
