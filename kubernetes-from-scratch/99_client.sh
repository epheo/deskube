#!/bin/bash
# Path: articles/kubernetes-from-scratch/08_operatorhub.sh

source _common.sh

sudo dnf -y install bash-completion

source <(kubectl completion bash)

echo "source <(kubectl completion bash)" >> ~/.bashrc

# Replaces line if it exists, otherwise appends to file
source <(kubectl completion bash | sed s/kubectl/k/g)
echo "source <(kubectl completion bash | sed s/kubectl/k/g)" >> ~/.bashrc

alias k=kubectl
echo "alias k=kubectl" >> ~/.bashrc

# Install k9s

url=https://github.com/derailed/k9s
version=$(github_latest ${url})

github_download ${url} ${version} k9s_Linux_amd64.tar.gz

tar -xvf k9s_Linux_amd64.tar.gz
sudo install k9s /usr/local/bin/
rm -f k9s_Linux_amd64.tar.gz k9s README.md LICENSE
