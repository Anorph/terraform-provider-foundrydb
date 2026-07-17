terraform {
  required_providers {
    foundrydb = {
      source = "anorph/foundrydb"
    }
  }
}

provider "foundrydb" {
  api_url  = "https://api.foundrydb.com"
  username = "admin"
  password = "admin"
}

# A highly available edge point-of-presence in Stockholm: a primary that holds
# the serving floating IP plus one hot standby.
resource "foundrydb_edge_node" "sto" {
  zone              = "se-sto1"
  target_node_count = 2
}

# A larger PoP in Helsinki with an explicit compute plan. Increasing
# target_node_count later scales the PoP in place; the edge state machine
# converges node_count toward the new target.
resource "foundrydb_edge_node" "hel" {
  zone              = "fi-hel1"
  plan              = "edge-tier-2"
  target_node_count = 3
}

output "sto_pop_id" {
  description = "UUID of the Stockholm edge PoP"
  value       = foundrydb_edge_node.sto.id
}

output "sto_pop_status" {
  description = "Lifecycle status of the Stockholm edge PoP"
  value       = foundrydb_edge_node.sto.status
}

output "sto_pop_live_node_count" {
  description = "Number of edge VMs currently live in the Stockholm PoP"
  value       = foundrydb_edge_node.sto.node_count
}
