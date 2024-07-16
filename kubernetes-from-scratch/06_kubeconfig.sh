#!/bin/bash
# Path: articles/kubernetes-from-scratch/kubeconfig.sh
# Title: Setting up the kubeconfig file

source _common.sh

kubectl config set-cluster ${cluster_name} \
  --certificate-authority=ca.pem \
  --embed-certs=true \
  --server=https://${ip_address}:6443

kubectl config set-credentials admin \
  --client-certificate=admin.pem \
  --client-key=admin-key.pem

kubectl config set-context ${cluster_name} \
  --cluster=${cluster_name} \
  --user=admin

kubectl config use-context ${cluster_name}

kubectl version

kubectl get nodes
