# Teleport Database

resource "teleport_database" "example" {
    metadata {
        name = "example"
        description = "Test database"
    }

    spec {
        protocol = "postgres"
        uri = "localhost"
    }
}