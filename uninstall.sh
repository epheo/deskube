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

dnf remove nginx
