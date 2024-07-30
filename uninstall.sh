#!/bin/bash


systemctl disable etcd
systemctl stop etcd

rm /usr/local/bin/etcd
rm /usr/local/bin/etcdctl
rm -rf /etc/etcd /var/lib/etcd

systemctl daemon-reload

echo "Uninstall etcd completed"

systemctl disable kube-apiserver kube-controller-manager kube-scheduler
systemctl stop kube-apiserver kube-controller-manager kube-scheduler

rm /usr/local/bin/kube-apiserver
rm /usr/local/bin/kube-controller-manager
rm /usr/local/bin/kube-scheduler
rm /usr/local/bin/kubectl

rm -rf /var/lib/kubernetes
rm -rf /etc/kubernetes

systemctl daemon-reload

echo "Uninstall kube-apiserver, kube-controller-manager, kube-scheduler completed"

dnf remove nginx -y

## Worker

systemctl disable kubelet kube-proxy containerd
systemctl stop kubelet kube-proxy containerd

rm /usr/local/bin/kubelet /usr/local/bin/kube-proxy /bin/containerd*

rm -rf /etc/cni /opt/cni /var/lib/kubelet /var/lib/kube-proxy /var/lib/kubernetes /var/run/kubernetes

systemctl daemon-reload

# sudo dnf remove -y socat conntrack container-selinux

echo "Uninstall kubelet, kube-proxy, containerd completed"

