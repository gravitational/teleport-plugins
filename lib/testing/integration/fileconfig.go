/*
Copyright 2021 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package integration

const teleportAuthYAML = `
teleport:
  data_dir: {{TELEPORT_DATA_DIR}}
  cache:
    enabled: false
  log:
    output: stdout
  storage:
    type: sqlite
    poll_stream_period: 50000000

auth_service:
  license_file: {{TELEPORT_LICENSE_FILE}}
  cluster_name: local-site
  enabled: true
  listen_addr: 127.0.0.1:0
  public_addr: localhost
  authentication:
    type: local
    second_factor: off

proxy_service:
  enabled: false

ssh_service:
  enabled: false
`
