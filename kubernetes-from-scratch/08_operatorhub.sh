#!/bin/bash
# Path: articles/kubernetes-from-scratch/08_operatorhub.sh

source _common.sh

url=https://github.com/operator-framework/operator-lifecycle-manager
version=$(github_latest ${url})

github_download ${url} ${version} install.sh

chmod +x install.sh
./install.sh v${version}