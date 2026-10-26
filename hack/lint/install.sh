#!/usr/bin/env bash
# Copyright 2019 Iguazio
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

set -e

echo Installing linters...

OS_NAME=$(uname)
FORCE_INSTALL=false
BIN_DIR=$(pwd)/.bin/
GOLANGCI_LINT_VERSION="2.12.1"

mkdir -p "${BIN_DIR}"

if [[ $# -ne 0 && "$1" == "force" ]]; then
    echo "Force install golangci-lint requested"
    FORCE_INSTALL=true
fi

if [[ -f "${BIN_DIR}/golangci-lint" ]]; then
    INSTALLED_VERSION=$("${BIN_DIR}/golangci-lint" version | awk '/version/ {gsub(/^v/, "", $4); print $4}')
    if [[ "${INSTALLED_VERSION}" != "${GOLANGCI_LINT_VERSION}" ]]; then
        echo "golangci-lint version mismatch (${INSTALLED_VERSION} != ${GOLANGCI_LINT_VERSION})"
        FORCE_INSTALL=true
    fi
fi

if [[ "${FORCE_INSTALL}" == true || ! -f "${BIN_DIR}/golangci-lint" ]]; then
    echo "golangci-lint binary does not exist or force install requested. Fetching and installing..."
    GOBIN="${BIN_DIR}" go install "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v${GOLANGCI_LINT_VERSION}"
    echo "golangci-lint installed in: ${BIN_DIR}/golangci-lint"
fi
