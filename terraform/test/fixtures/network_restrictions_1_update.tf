# Teleport Network Restrictions

resource "teleport_network_restrictions" "example" {
  metadata = {
    description = "network restrictions"
  }

  spec = {
    allow = [{
      cidr = "192.168.0.0/16"
    }]
    deny = [{
      cidr = "101.101.2.4"
    }]
  }
}
