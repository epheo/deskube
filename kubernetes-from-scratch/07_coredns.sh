#!/bin/bash

source _common.sh

curl https://raw.githubusercontent.com/helm/helm/master/scripts/get-helm-3 | bash

helm repo add coredns https://coredns.github.io/helm
helm --namespace=kube-system install coredns coredns/coredns --set service.clusterIP="${cluster_dns}"

# kubectl run -it --rm --restart=Never --image=infoblox/dnstools:latest dnstools

