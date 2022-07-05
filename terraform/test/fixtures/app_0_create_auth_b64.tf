resource "teleport_app" "test_auth_b64" {
    metadata = {
        name = "test_auth_b64"
        description = "Test app"
        labels  = {
            example = "yes"
            "teleport.dev/origin" = "dynamic"
        }    
    }

    spec = {
        uri = "localhost:3000"
    }
}