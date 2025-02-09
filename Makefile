#
# Copyright 2021 VMware, Inc..
# SPDX-License-Identifier: Apache-2.0
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

export GO111MODULE=on
export GOFLAGS=-mod=readonly
export CGO_CFLAGS="-DVer=8.0"
#export VDDK_PATH="/usr/lib/vmware-vix-disklib.8.0"
export CGO_CFLAGS_ALLOW="-I/usr/lib/vmware-vix-disklib.8.0/include -std=c99"
export CGO_LDFLAGS_ALLOW="-L/usr/lib/vmware-vix-disklib.8.0/lib64 -lvixDiskLib"

all: build

build: disklib virtual_disks

disklib: 
	cd pkg/disklib; go build

virtual_disks: 
	cd pkg/virtual_disks; go build
