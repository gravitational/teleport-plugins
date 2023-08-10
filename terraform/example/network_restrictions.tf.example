# Teleport Network Restrictions

resource "teleport_network_restrictions" "example" {
  metadata = {
    description = "network restrictions"
  }

  spec = {
    allow = [{
      cidr = "127.0.0.0/8"
    }]
    deny = [{
      cidr = "10.1.2.4"
    }]
  }
}
