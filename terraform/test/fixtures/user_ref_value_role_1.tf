resource "teleport_role" "test_ref_value" {
    metadata = {
        name = "test_ref_value"
    }

    spec = {
        allow = {
            logins = ["root"]
        }
    }
}

resource "teleport_user" "test_ref_value" {    
    metadata = {
        name    = "test_ref_value"
    }
    
    spec = {
        roles = [ (teleport_role.test_ref_value).metadata.name ]
    }
}

