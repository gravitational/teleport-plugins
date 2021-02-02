terraform {
  required_providers {
    teleport = {
      version = "0.0.1"
      source = "gravitational.com/teleport/teleport"
    }
  }
}

provider "teleport" {
    addr = "teleport.cluster.local:3025"
    cert_path = "/mnt/shared/certs/access-plugin/auth.crt"
    key_path = "/mnt/shared/certs/access-plugin/auth.key"
    root_ca_path = "/mnt/shared/certs/access-plugin/auth.cas"
}

resource "teleport_user" "tf_test" {

}