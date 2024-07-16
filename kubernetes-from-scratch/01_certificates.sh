#!/bin/bash
# Path: articles/kubernetes-from-scratch/certificates.sh

source _common.sh

# Installing cfssl and cfssljson

uninstall_cfssl() {
    sudo rm -f /usr/local/bin/cfssl /usr/local/bin/cfssljson
}

url=https://github.com/cloudflare/cfssl
version=$(github_latest ${url})

github_download ${url} ${version} cfssl_${version}_linux_amd64
github_download ${url} ${version} cfssljson_${version}_linux_amd64

mv cfssl_${version}_linux_amd64 cfssl
mv cfssljson_${version}_linux_amd64 cfssljson

chmod +x cfssl cfssljson
sudo install cfssl /usr/local/bin/
sudo install cfssljson /usr/local/bin/

# Generating the CA configuration file, certificate, and private key

cat > ca-config.json <<EOF
{
  "signing": {
    "default": {
      "expiry": "8760h"
    },
    "profiles": {
      "kubernetes": {
        "usages": ["signing", "key encipherment", "server auth", "client auth"],
        "expiry": "8760h"
      }
    }
  }
}
EOF

cat > ca-csr.json <<EOF
{
  "CN": "Kubernetes",
  "key": {
    "algo": "rsa",
    "size": 2048
  },
  "names": [{
    "C": "AQ",
    "L": "Antartica",
    "O": "Kubernetes",
    "OU": "SP",
    "ST": "South Pole"
  }]
}
EOF

if [ ! -f ca.pem ]; then
    cfssl gencert -initca ca-csr.json |cfssljson -bare ca
    rm ca.csr
fi

rm ca-csr.json

cat > admin-csr.json <<EOF
{
  "CN": "admin",
  "key": {
    "algo": "rsa",
    "size": 2048
  },
  "names": [
    {
      "C": "AQ",
      "L": "Antartica",
      "O": "system:masters",
      "OU": "Kubernetes",
      "ST": "South Pole"
    }
  ]
}
EOF

cfssl gencert \
  -ca=ca.pem \
  -ca-key=ca-key.pem \
  -config=ca-config.json \
  -profile=kubernetes \
  admin-csr.json | cfssljson -bare admin

rm admin.csr admin-csr.json

# Generating the kubelet client certificates

instance=${worker_hostname}

cat > ${instance}-csr.json <<EOF
{
  "CN": "system:node:${instance}",
  "key": {
    "algo": "rsa",
    "size": 2048
  },
  "names": [
    {
      "C": "AQ",
      "L": "Antartica",
      "O": "system:nodes",
      "OU": "Kubernetes",
      "ST": "South Pole"
    }
  ]
}
EOF

cfssl gencert \
  -ca=ca.pem \
  -ca-key=ca-key.pem \
  -config=ca-config.json \
  -hostname=${instance},${ip_address} \
  -profile=kubernetes \
  ${instance}-csr.json | cfssljson -bare ${instance}

rm ${instance}.csr ${instance}-csr.json

# Generating the kube-controller-manager client certificate

cat > kube-controller-manager-csr.json <<EOF
{
  "CN": "system:kube-controller-manager",
  "key": {
    "algo": "rsa",
    "size": 2048
  },
  "names": [
    {
      "C": "AQ",
      "L": "Antartica",
      "O": "system:kube-controller-manager",
      "OU": "Kubernetes",
      "ST": "South Pole"
    }
  ]
}
EOF

cfssl gencert \
  -ca=ca.pem \
  -ca-key=ca-key.pem \
  -config=ca-config.json \
  -profile=kubernetes \
  kube-controller-manager-csr.json | cfssljson -bare kube-controller-manager

rm kube-controller-manager.csr kube-controller-manager-csr.json

# Generating the kube-proxy client certificate

cat > kube-proxy-csr.json <<EOF
{
  "CN": "system:kube-proxy",
  "key": {
    "algo": "rsa",
    "size": 2048
  },
  "names": [
    {
      "C": "AQ",
      "L": "Antartica",
      "O": "system:node-proxier",
      "OU": "Kubernetes",
      "ST": "South Pole"
    }
  ]
}
EOF

cfssl gencert \
  -ca=ca.pem \
  -ca-key=ca-key.pem \
  -config=ca-config.json \
  -profile=kubernetes \
  kube-proxy-csr.json | cfssljson -bare kube-proxy

rm kube-proxy.csr kube-proxy-csr.json

# Generating the kube-scheduler client certificate

cat > kube-scheduler-csr.json <<EOF
{
  "CN": "system:kube-scheduler",
  "key": {
    "algo": "rsa",
    "size": 2048
  },
  "names": [
    {
      "C": "AQ",
      "L": "Antartica",
      "O": "system:kube-scheduler",
      "OU": "Kubernetes",
      "ST": "South Pole"
    }
  ]
}
EOF

cfssl gencert \
  -ca=ca.pem \
  -ca-key=ca-key.pem \
  -config=ca-config.json \
  -profile=kubernetes \
  kube-scheduler-csr.json | cfssljson -bare kube-scheduler

rm kube-scheduler.csr kube-scheduler-csr.json

# Generating the Kubernetes API Server certificate

cat > kubernetes-csr.json <<EOF
{
  "CN": "kubernetes",
  "key": {
    "algo": "rsa",
    "size": 2048
  },
  "names": [
    {
      "C": "AQ",
      "L": "Antartica",
      "O": "Kubernetes",
      "OU": "Kubernetes",
      "ST": "South Pole"
    }
  ]
}
EOF

kubernetes_hostnames=kubernetes,kubernetes.default,kubernetes.default.svc,kubernetes.default.svc.cluster,kubernetes.svc.${cluster_domain}

kubernetes_hostnames="${kubernetes_hostnames},${ip_address}",127.0.0.1,${cluster_ip}

cfssl gencert \
  -ca=ca.pem \
  -ca-key=ca-key.pem \
  -config=ca-config.json \
  -hostname=${kubernetes_hostnames} \
  -profile=kubernetes \
  kubernetes-csr.json | cfssljson -bare kubernetes

rm kubernetes.csr kubernetes-csr.json

#Generating the service-account certificate

cat > service-account-csr.json <<EOF
{
  "CN": "service-accounts",
  "key": {
    "algo": "rsa",
    "size": 2048
  },
  "names": [
    {
      "C": "AQ",
      "L": "Antartica",
      "O": "Kubernetes",
      "OU": "Kubernetes",
      "ST": "South Pole"
    }
  ]
}
EOF

cfssl gencert \
  -ca=ca.pem \
  -ca-key=ca-key.pem \
  -config=ca-config.json \
  -profile=kubernetes \
  service-account-csr.json | cfssljson -bare service-account

rm service-account.csr service-account-csr.json

# Get back to the root directory as the next script will be executed from there and
# _common.sh cd's into the cluster directory
cd -