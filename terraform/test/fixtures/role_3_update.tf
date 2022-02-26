resource "teleport_role" "test" {
    metadata = {
        name = "test"
        expires = "2022-12-12T00:00:00Z"
    }

    spec = {
        allow = {
            logins = ["anonymous"]
            request = {}
            node_labels = {}
        }
    }

    version = "v4"
}
