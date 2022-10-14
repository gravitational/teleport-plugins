resource "teleport_oidc_connector" "test_multiple_redirects" {
    metadata = {
        name    = "test_multiple_redirects"
        expires = "2032-10-12T07:20:50Z"
        labels  = {
            example = "yes"
        }
    }

    spec = {
        client_id = "client"
        client_secret = "value"
    
        claims_to_roles = [{
            claim = "test"
            roles = ["terraform"]
        }]

        redirect_urls = [ "https://example.com/redirect", "https://example.com/redirect2" ]
    }
}
