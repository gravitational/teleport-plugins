resource "teleport_app" "test_without_cache" {
    metadata = {
        name = "example"
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