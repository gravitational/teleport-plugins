# Teleport App

resource "teleport_app" "example" {
    metadata {
        name = "example"
        description = "Test app"
    }

    spec {
        uri = "localhost:3000"
    }
}