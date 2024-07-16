#!/bin/bash
# Path: articles/kubernetes-from-scratch/etcd.sh

source _common.sh

sudo dnf install tar -y

# Installing etcd

uninstall_etcd() {
    sudo systemctl stop etcd
    sudo systemctl disable etcd
    sudo rm -f /etc/systemd/system/etcd.service
    sudo rm -f /usr/local/bin/etcd /usr/local/bin/etcdctl
    sudo rm -rf /etc/etcd /var/lib/etcd
}

url=https://github.com/etcd-io/etcd
version=$(github_latest ${url})

github_download ${url} ${version} etcd-v${version}-linux-amd64.tar.gz

tar xvf etcd-v${version}-linux-amd64.tar.gz && rm -f etcd-v${version}-linux-amd64.tar.gz

sudo install etcd-v${version}-linux-amd64/etcd* /usr/local/bin/
rm -rf etcd-v${version}-linux-amd64

sudo /sbin/restorecon -v /usr/local/bin/etcd # is this still needed ?
sudo /sbin/restorecon -v /usr/local/bin/etcdctl

sudo mkdir -p /etc/etcd /var/lib/etcd
sudo chmod 700 /var/lib/etcd
sudo cp ca.pem kubernetes-key.pem kubernetes.pem /etc/etcd/

# Setting up the etcd Server

cat <<EOF | sudo tee /etc/systemd/system/etcd.service
[Unit]
Description=etcd
Documentation=https://github.com/coreos

[Service]
Type=notify
ExecStart=/usr/local/bin/etcd \\
  --name ${hostname} \\
  --cert-file=/etc/etcd/kubernetes.pem \\
  --key-file=/etc/etcd/kubernetes-key.pem \\
  --peer-cert-file=/etc/etcd/kubernetes.pem \\
  --peer-key-file=/etc/etcd/kubernetes-key.pem \\
  --trusted-ca-file=/etc/etcd/ca.pem \\
  --peer-trusted-ca-file=/etc/etcd/ca.pem \\
  --peer-client-cert-auth \\
  --client-cert-auth \\
  --initial-advertise-peer-urls https://${ip_address}:2380 \\
  --listen-peer-urls https://${ip_address}:2380 \\
  --listen-client-urls https://${ip_address}:2379,https://127.0.0.1:2379 \\
  --advertise-client-urls https://${ip_address}:2379 \\
  --initial-cluster-token etcd-cluster-0 \\
  --initial-cluster ${hostname}=https://${ip_address}:2380 \\
  --initial-cluster-state new \\
  --data-dir=/var/lib/etcd
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

# Starting up the etcd Server

sudo systemctl daemon-reload
sudo systemctl enable etcd
sudo systemctl restart etcd

# Verification

sudo ETCDCTL_API=3 /usr/local/bin/etcdctl member list \
  --endpoints=https://${ip_address}:2379 \
  --cacert=/etc/etcd/ca.pem \
  --cert=/etc/etcd/kubernetes.pem \
  --key=/etc/etcd/kubernetes-key.pem

# Get back to the root directory as the next script will be executed from there and
# _common.sh cd's into the cluster directory
cd -