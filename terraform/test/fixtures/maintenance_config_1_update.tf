resource "teleport_cluster_maintenance_config" "test" {
  metadata = {
    description = "Maintenance config"
  }

  spec = {
	agent_upgrades = {
	  utc_start_hour = 12
	  weekdays = [ "tuesday" ]
	}
  }
}
