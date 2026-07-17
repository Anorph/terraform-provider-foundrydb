# foundrydb_edge_node

Provisions and scales an edge point-of-presence (PoP) in the FoundryDB edge tier.

A PoP is one or more edge VMs in a single zone. A `target_node_count` of `2` or
more runs a primary that holds the serving floating IP plus one or more hot
standbys, giving the PoP high availability. The edge state machine converges the
live `node_count` toward `target_node_count` asynchronously.

Changing `target_node_count` scales the PoP in place. Changing `zone` or `plan`
destroys and recreates it.

This resource models the declarative surface of the edge fleet only. The
imperative roll operation and the read-only fleet overview and recovery views are
handled through the SDK and CLI, not through Terraform.

## Example Usage

```hcl
# A highly available edge PoP: primary + one hot standby.
resource "foundrydb_edge_node" "sto" {
  zone              = "se-sto1"
  target_node_count = 2
}

# A larger PoP with an explicit compute plan.
resource "foundrydb_edge_node" "hel" {
  zone              = "fi-hel1"
  plan              = "edge-tier-2"
  target_node_count = 3
}
```

## Arguments

| Argument | Type | Required | Forces Replace | Description |
|----------|------|----------|----------------|-------------|
| `zone` | string | Yes | Yes | Cloud zone the PoP is provisioned in (e.g. `se-sto1`). |
| `plan` | string | No | Yes | Compute plan for the PoP's edge VMs. When omitted, the platform selects the default edge plan. |
| `target_node_count` | number | No | No | Desired number of edge VMs in the PoP. `1` is a single-node PoP; `2` or more provisions a primary plus hot standbys. Changing this value scales the PoP in place. Default: `2`. |

## Computed Attributes

| Attribute | Description |
|-----------|-------------|
| `id` | UUID of the edge PoP. |
| `name` | Platform-assigned name of the PoP. |
| `status` | Current lifecycle status as reported by the edge state machine (e.g. `provisioning`, `running`, `scaling`, `failed`). |
| `node_count` | Number of edge VMs currently live in the PoP. Converges toward `target_node_count` as the edge state machine reconciles. |

## Provisioning behaviour

On create, the provider submits the PoP and then polls the fleet until the PoP
reaches a running state, up to a bounded timeout of 20 minutes. Scaling
(`target_node_count` change) returns as soon as the desired count is accepted;
the edge state machine adds or removes VMs in the background, and a subsequent
`terraform plan` reflects the converged `node_count`.
