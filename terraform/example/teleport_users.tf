resource "teleport_user" "nate" {
    metadata {
        name = "Nate"
        description = "Test user via Terraform"
    }
}