#!/bin/bash
# Path: articles/kubernetes-from-scratch/_common.sh
# Title: Common functions for Kubernetes from Scratch
set -euo pipefail

cluster_name="kubernetesh"

#check if curl version less than 7.87.0
curl_version=$(curl --version | head -n 1 | cut -d ' ' -f 2)

if [[ $curl_version < 7.87.0 ]]; then
    echo "curl version is less than 7.87.0"
    echo "Please upgrade curl to version 7.87.0 or higher"
    echo "Will download static curl version 8.4.0"
    curl -L https://github.com/moparisthebest/static-curl/releases/download/v8.4.0/curl-amd64 > curl && chmod +x curl
    curl="$(pwd)/curl -k" # Setting -k as new curl has different location for cacert
else
    curl=$(which curl)
fi

mkdir -p ${cluster_name} && cd ${cluster_name}

github_latest() {
    $curl --silent -w "%header{location}\n" ${1}/releases/latest |awk -F "/" '{print $NF}' |sed s/v//
}

github_download() {
    local url=${1}
    local version=${2}
    local file=${3}
    local download_url=${url}/releases/download/v${version}/${file}
    curl -L -o ${file} ${download_url}
}

# Find the main network interface dynamically
main_interface=$(ip route | awk '/default/ {print $5}' | head -1)

echo "Main interface is ${main_interface}"

# Get the IP address and subnet of the main network interface
ip_address=$(ip -4 addr show "$main_interface" | awk '/inet/ {print $2}' | cut -d'/' -f1)
subnet=$(ip -4 addr show "$main_interface" | awk '/inet/ {print $2}' | cut -d'/' -f2)
echo "IP address is ${ip_address} and subnet is ${subnet}"

# the range of IP addresses used for pods
cluster_network="10.200.0.0/16"
cluster_domain="cluster.local"

# the range of IP addresses used for service ClusterIPs
service_network="10.32.0.0/24"
cluster_ip="10.32.0.1"
cluster_dns="10.32.0.10"

# Get the hostname
hostname=$(hostname -s)
worker_hostname=$(hostname -s)

echo "Hostname is ${hostname}"
