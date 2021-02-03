terraform {
  required_providers {
    teleport = {
      version = "0.0.1"
      source = "gravitational.com/teleport/teleport"
    }
  }
}

provider "teleport" {
    addr = "localhost:3025"
    cert_path = "/Users/xnutsive/go/src/github.com/gravitational/teleport-plugins/docker/tmp/auth.crt"
    key_path = "/Users/xnutsive/go/src/github.com/gravitational/teleport-plugins/docker/tmp/auth.key"
    root_ca_path = "/Users/xnutsive/go/src/github.com/gravitational/teleport-plugins/docker/tmp/auth.cas"
}

resource "teleport_user" "tf_test" {

  metadata {
    name = "Nate"
    description = "Terraform Test User"

    # This will probably explode
    expires = "2022-10-12T07:20:50.52Z"

    labels = {
      test = "label value"
    }
  }

  spec {
    roles = ["foo"]

    # TODO add Traits and Identities.
    # Traits schema is fucked.
  }

}