// Copyright 2020 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// +build linux
<<<<<<< HEAD
// +build mips mipsle mips64 mips64le

package procfs

var parseCPUInfo = parseCPUInfoMips
=======
<<<<<<< HEAD:vendor/github.com/prometheus/procfs/cpuinfo_others.go
// +build !386,!amd64,!arm,!arm64,!mips,!mips64,!mips64le,!mipsle,!ppc64,!ppc64le,!riscv64,!s390x
=======
// +build mips mipsle mips64 mips64le
>>>>>>> 9675161e (Upgrade Teleport dependency to v6):vendor/github.com/prometheus/procfs/cpuinfo_mipsx.go

package procfs

var parseCPUInfo = parseCPUInfoDummy
>>>>>>> 9675161e (Upgrade Teleport dependency to v6)
