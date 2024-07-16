#!/bin/bash
# Path: articles/kubernetes-from-scratch/kubectl.sh

source _common.sh

#Installing kubectl

uninstall_kubectl() {
    sudo rm -f /usr/local/bin/kubectl
}

url=https://storage.googleapis.com/kubernetes-release/release
version=$(curl -s ${url}/stable.txt)

curl -LO ${url}/${version}/bin/linux/amd64/kubectl

chmod +x kubectl
sudo install kubectl /usr/local/bin/

# Setting up the Kubernetes Worker Node

instance=${worker_hostname}

kubectl config set-cluster ${cluster_name} \
  --certificate-authority=ca.pem \
  --embed-certs=true \
  --server=https://${ip_address}:6443 \
  --kubeconfig=${instance}.kubeconfig

kubectl config set-credentials system:node:${instance} \
  --client-certificate=${instance}.pem \
  --client-key=${instance}-key.pem \
  --embed-certs=true \
  --kubeconfig=${instance}.kubeconfig

kubectl config set-context default \
  --cluster=${cluster_name} \
  --user=system:node:${instance} \
  --kubeconfig=${instance}.kubeconfig

kubectl config use-context default --kubeconfig=${instance}.kubeconfig

# Setting up the Kubernetes Proxy

kubectl config set-cluster ${cluster_name} \
  --certificate-authority=ca.pem \
  --embed-certs=true \
  --server=https://${ip_address}:6443 \
  --kubeconfig=kube-proxy.kubeconfig

kubectl config set-credentials system:kube-proxy \
  --client-certificate=kube-proxy.pem \
  --client-key=kube-proxy-key.pem \
  --embed-certs=true \
  --kubeconfig=kube-proxy.kubeconfig

kubectl config set-context default \
  --cluster=${cluster_name} \
  --user=system:kube-proxy \
  --kubeconfig=kube-proxy.kubeconfig

kubectl config use-context default --kubeconfig=kube-proxy.kubeconfig

# Generate a kubeconfig file for the kube-controller-manager service

kubectl config set-cluster ${cluster_name} \
  --certificate-authority=ca.pem \
  --embed-certs=true \
  --server=https://127.0.0.1:6443 \
  --kubeconfig=kube-controller-manager.kubeconfig

kubectl config set-credentials system:kube-controller-manager \
  --client-certificate=kube-controller-manager.pem \
  --client-key=kube-controller-manager-key.pem \
  --embed-certs=true \
  --kubeconfig=kube-controller-manager.kubeconfig

kubectl config set-context default \
  --cluster=${cluster_name} \
  --user=system:kube-controller-manager \
  --kubeconfig=kube-controller-manager.kubeconfig

kubectl config use-context default --kubeconfig=kube-controller-manager.kubeconfig

# Generate a kubeconfig file for the kube-scheduler service

kubectl config set-cluster ${cluster_name} \
  --certificate-authority=ca.pem \
  --embed-certs=true \
  --server=https://127.0.0.1:6443 \
  --kubeconfig=kube-scheduler.kubeconfig

kubectl config set-credentials system:kube-scheduler \
  --client-certificate=kube-scheduler.pem \
  --client-key=kube-scheduler-key.pem \
  --embed-certs=true \
  --kubeconfig=kube-scheduler.kubeconfig

kubectl config set-context default \
  --cluster=${cluster_name} \
  --user=system:kube-scheduler \
  --kubeconfig=kube-scheduler.kubeconfig

kubectl config use-context default --kubeconfig=kube-scheduler.kubeconfig

# Generate a kubeconfig file for the admin user

kubectl config set-cluster ${cluster_name} \
  --certificate-authority=ca.pem \
  --embed-certs=true \
  --server=https://127.0.0.1:6443 \
  --kubeconfig=admin.kubeconfig

kubectl config set-credentials admin \
  --client-certificate=admin.pem \
  --client-key=admin-key.pem \
  --embed-certs=true \
  --kubeconfig=admin.kubeconfig

kubectl config set-context default \
  --cluster=${cluster_name} \
  --user=admin \
  --kubeconfig=admin.kubeconfig

kubectl config use-context default --kubeconfig=admin.kubeconfig


if [ ! -f encryption-config.yaml ]; then
# Create the encryption-config.yaml encryption config file:
echo "Encryption key not found, generating a new one"
ENCRYPTION_KEY=$(head -c 32 /dev/urandom | base64)
cat > encryption-config.yaml <<EOF
kind: EncryptionConfig
apiVersion: v1
resources:
  - resources:
      - secrets
    providers:
      - aescbc:
          keys:
            - name: key1
              secret: ${ENCRYPTION_KEY}
      - identity: {}
EOF
fi


# Get back to the root directory as the next script will be executed from there and
# _common.sh cd's into the cluster directory
cd -