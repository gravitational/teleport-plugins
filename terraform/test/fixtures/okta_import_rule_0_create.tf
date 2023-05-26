resource "teleport_okta_import_rule" "test" {
  metadata = {
    name        = "example"
    description = "Test Okta Import Rule"
    labels = {
      example               = "yes"
      "teleport.dev/origin" = "okta"
    }
  }

  spec = {
    priority = 100
    mappings = [
      {
        add_labels = {
          "label1" : "value1",
        }
        matches = [
          {
            app_ids = ["1", "2", "3"]
          },
        ],
      },
      {
        add_labels = {
          "label2" : "value2",
        }
        matches = [
          {
            group_ids = ["1", "2", "3"]
          },
        ],
      }
    ]
  }
}
