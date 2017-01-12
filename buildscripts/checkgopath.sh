#!/usr/bin/env bash
#
# MinFS - fuse driver for Object Storage (C) 2016, 2017 Minio, Inc.
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

main() {
    echo "Checking project is in GOPATH:"

    IFS=':' read -r -a paths <<< "$GOPATH"
    for path in "${paths[@]}"; do
        minio_path="$path/src/github.com/minio/minfs"
        if [ -d "$minio_path" ]; then
            if [ "$minio_path" -ef "$PWD" ]; then
               exit 0
            fi
        fi
    done

    echo "ERROR"
    echo "Project not found in ${GOPATH}."
    echo "Follow instructions at https://github.com/minio/minfs/blob/master/CONTRIBUTING.md#setup-your-minfs-github-repository"
    exit 1
}

main

EOF
_init() {

    shopt -s extglob

    # Fetch real paths instead of symlinks before comparing them
    PWD=$(env pwd -P)
    GOPATH=$(cd "$(go env GOPATH)" ; env pwd -P)
}

main() {
    echo "Checking if project is at ${GOPATH}"
    for minfs in $(echo ${GOPATH} | tr ':' ' '); do
        if [ ! -d ${minfs}/src/github.com/minio/minfs ]; then
            echo "Project not found in ${minfs}, please follow instructions provided at https://github.com/minio/minfs/blob/master/CONTRIBUTING.md#setup-your-minfs-github-repository" \
                && exit 1
        fi
        if [ "x${minfs}/src/github.com/minio/minfs" != "x${PWD}" ]; then
            echo "Build outside of ${minfs}, two source checkouts found. Exiting." && exit 1
        fi
    done
}

_init && main

