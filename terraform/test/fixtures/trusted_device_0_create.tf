
resource "teleport_trusted_device" "TESTDEVICE" {
  spec = {
    asset_tag     = "TESTDEVICE"
    os_type       = "macos"
    enroll_status = "not_enrolled"
    collected_data = [
      {
        collect_time  = "2024-10-12T07:20:50Z"
        record_time   = "2024-10-12T07:20:50Z"
        os_type       = "macos"
        serial_number = "TESTDEVICE"
    }]
  }
}
