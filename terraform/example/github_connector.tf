# Terraform Github connector

variable "github_secret" {}

resource "teleport_github_connector" "github" {
  # This section tells Terraform that role example must be created before the GitHub connector
  depends_on = [
    teleport_role.example
  ]

  metadata {
     name = "example"
     labels = {
       example = "yes"
     }
  }
  
  spec {
    client_id = " Iv1.3386eee92ff932a4"
    client_secret = var.github_secret

    teams_to_logins {
       organization = "evilmartians"
       team = "devs"
       logins = ["example"]

       # Please, provide this values explicitly, event if empty 
       kubernetes_groups = []
       kubernetes_users = []
    }
  }
}
