# Terraform Provider for FoundryDB

Manage [FoundryDB](https://foundrydb.com) managed database clusters with Terraform. This provider supports PostgreSQL, MySQL, MongoDB, Valkey, Kafka, OpenSearch, and MSSQL running on UpCloud infrastructure.

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) 1.0+
- [Go](https://golang.org/doc/install) 1.24+ (to build the provider from source)
- FoundryDB account with API access

## Installation

### From Terraform Registry

Add the provider to your Terraform configuration:

```hcl
terraform {
  required_providers {
    foundrydb = {
      source  = "anorph/foundrydb"
      version = "~> 1.0"
    }
  }
}
```

Then run `terraform init`.

### Build from Source

```bash
git clone https://github.com/anorph/terraform-provider-foundrydb.git
cd terraform-provider-foundrydb
CGO_ENABLED=0 go build -o terraform-provider-foundrydb .
```

Place the built binary in your [plugin directory](https://developer.hashicorp.com/terraform/cli/config/config-file#implied-local-mirror-directories).

## Provider Configuration

```hcl
provider "foundrydb" {
  api_url  = "https://api.foundrydb.com"   # optional, this is the default
  username = "admin"
  password = "admin"
}
```

### Provider Arguments

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| `api_url` | string | No | Base URL of the FoundryDB API. Defaults to `https://api.foundrydb.com`. |
| `username` | string | Yes | Basic Auth username. |
| `password` | string | Yes | Basic Auth password. |

Use environment variables or a secrets manager to avoid hardcoding credentials.

## Resources

### `foundrydb_service`

Creates and manages a managed database service. After creation, the provider polls until the service reaches `running` status (up to 15 minutes).

#### Example

```hcl
resource "foundrydb_service" "postgres" {
  name            = "prod-pg"
  database_type   = "postgresql"
  version         = "17"
  plan_name       = "tier-2"
  zone            = "se-sto1"
  storage_size_gb = 50
  storage_tier    = "maxiops"
  allowed_cidrs   = ["0.0.0.0/0"]
}
```

#### Arguments

| Argument | Type | Required | Forces Replace | Description |
|----------|------|----------|----------------|-------------|
| `name` | string | Yes | No | Human-readable service name. |
| `database_type` | string | Yes | Yes | Engine: `postgresql`, `mysql`, `mongodb`, `valkey`, `kafka`, `opensearch`, `mssql`. |
| `version` | string | No | Yes | Engine version (see [Supported Versions](#supported-versions)). |
| `plan_name` | string | Yes | Yes | Compute tier: `tier-1` to `tier-15`. |
| `zone` | string | No | Yes | UpCloud zone. Default: `se-sto1` (Stockholm). |
| `storage_size_gb` | number | No | Yes | Data disk size in GB. |
| `storage_tier` | string | No | Yes | `maxiops` (NVMe SSD, production) or `standard` (HDD, development). |
| `allowed_cidrs` | list(string) | No | No | CIDR blocks allowed to connect to the service. |
| `organization_id` | string | No | Yes | Organization ID to scope this service to. When set, all API calls for this resource include the `X-Active-Org-ID` header. Use with the `foundrydb_organizations` data source to look up org IDs. |

#### Computed Attributes

| Attribute | Description |
|-----------|-------------|
| `id` | UUID of the service. |
| `status` | Current lifecycle status (e.g. `running`). |
| `created_at` | RFC3339 creation timestamp. |

#### Supported Versions

| Engine | Supported Versions |
|--------|-------------------|
| `postgresql` | `14`, `15`, `16`, `17`, `18` |
| `mysql` | `8.4` |
| `mongodb` | `6.0`, `7.0`, `8.0` |
| `valkey` | `7.2`, `8.0`, `8.1`, `9.0` |
| `kafka` | `3.6`, `3.7`, `3.8`, `3.9`, `4.0` |
| `opensearch` | `2` |
| `mssql` | `4.8` |

#### Compute Tiers

| Tier | vCPUs | Memory |
|------|-------|--------|
| tier-1 | 1 | 4 GB |
| tier-2 | 2 | 4 GB |
| tier-3 | 2 | 8 GB |
| tier-4 | 4 | 8 GB |
| tier-5 | 4 | 16 GB |
| tier-6 | 8 | 32 GB |
| tier-7 | 8 | 64 GB |
| tier-8 | 16 | 64 GB |
| tier-9 | 20 | 96 GB |
| tier-10 | 24 | 128 GB |
| tier-11 | 32 | 128 GB |
| tier-12 | 40 | 160 GB |
| tier-13 | 48 | 224 GB |
| tier-14 | 64 | 256 GB |
| tier-15 | 80 | 512 GB |

#### In-place Updates

Only `name` and `allowed_cidrs` can be updated without recreating the service. All other arguments force replacement.

### `foundrydb_app_job`

Creates and manages a job definition on a FoundryDB app service. A job is a container run (image, command, and environment layered over the app's own configuration) with an optional cron schedule. Jobs without a schedule run only on explicit invocation via the platform API.

#### Example

```hcl
resource "foundrydb_app_job" "nightly_cleanup" {
  app_service_id = foundrydb_service.app.id
  name           = "nightly-cleanup"
  schedule_cron  = "0 2 * * *"
  timezone       = "Europe/Stockholm"
  enabled        = true

  image_ref = "registry.example.com/tools:cleanup-v2"
  command   = ["/app/cleanup", "--older-than=30d"]

  env = {
    CLEANUP_DRY_RUN = "false"
    LOG_LEVEL       = "info"
  }

  max_retries           = 2
  retry_backoff_seconds = 60
  max_runtime_seconds   = 1800
  concurrency_cap       = 1
}
```

#### Arguments

| Argument | Type | Required | Forces Replace | Description |
|----------|------|----------|----------------|-------------|
| `app_service_id` | string | Yes | Yes | UUID of the app service that owns this job. |
| `name` | string | Yes | Yes | Unique name for the job within the app service. |
| `schedule_cron` | string | No | No | Five-field cron expression (minute granularity; descriptors such as `@daily` accepted) evaluated in `timezone`. Omit for an unscheduled job. Removing this field from config sends `clear_schedule` to the API. |
| `timezone` | string | No | No | IANA timezone name for cron evaluation (e.g. `UTC`, `Europe/Stockholm`). Default: `UTC`. |
| `enabled` | bool | No | No | Whether the schedule is active. Disabled jobs still run on explicit invocation. Default: `true`. |
| `image_ref` | string | No | No | Container image reference override (e.g. `registry.example.com/tools:latest`). Omit to inherit the app's image. Removing this field sends `clear_image_ref` to the API. |
| `command` | list(string) | No | No | Container argv override in exec form. Omit to use the image default. |
| `env` | map(string) | No | No | Environment variables layered over the app's environment at dispatch time. Job keys override app keys. |
| `max_retries` | number | No | No | Retry count after failure before the invocation is permanently failed. Default: `0`. |
| `retry_backoff_seconds` | number | No | No | Minimum delay in seconds between retry attempts. Default: `0`. |
| `max_runtime_seconds` | number | No | No | Maximum wall-clock time before the platform terminates the invocation. Default: `3600`. |
| `concurrency_cap` | number | No | No | Maximum simultaneous invocations. A new invocation exceeding this cap is rejected. Default: `1`. |

#### Computed Attributes

| Attribute | Description |
|-----------|-------------|
| `id` | UUID of the job. |

#### In-place Updates

All arguments except `app_service_id` and `name` can be updated without recreating the job. Removing `schedule_cron` sends `clear_schedule`; removing `image_ref` sends `clear_image_ref`.

---

### `foundrydb_queue`

Creates and manages a message queue on a FoundryDB PostgreSQL managed service. Queue state (messages) lives in the customer's database, transactional with their data. After creation, the provider polls until the queue reaches `Active` status. All arguments are immutable after creation; any change destroys and recreates the queue.

#### Example

```hcl
resource "foundrydb_queue" "tasks" {
  service_id                 = foundrydb_service.pg.id
  name                       = "tasks"
  visibility_timeout_seconds = 30
  max_attempts               = 5
  dlq_enabled                = true
}
```

#### Arguments

| Argument | Type | Required | Forces Replace | Description |
|----------|------|----------|----------------|-------------|
| `service_id` | string | Yes | Yes | UUID of the PostgreSQL managed service that hosts this queue. |
| `name` | string | Yes | Yes | Unique name for the queue within the service. |
| `visibility_timeout_seconds` | number | No | Yes | Redelivery horizon in seconds: how long a claimed message stays invisible before a crashed consumer's claim expires. Default: `30`. |
| `max_attempts` | number | No | Yes | Maximum delivery attempts before a message is dropped or dead-lettered. Default: `5`. |
| `dlq_enabled` | bool | No | Yes | Whether exhausted messages are moved to a dead-letter queue instead of being dropped. Default: `true`. |

#### Computed Attributes

| Attribute | Description |
|-----------|-------------|
| `id` | UUID of the queue. |
| `status` | Current lifecycle status: `Pending`, `Provisioning`, `Active`, `Deprovisioning`, or `Failed`. |
| `database_name` | Name of the customer database where queue schema objects are created. |

---

### `foundrydb_edge_node`

Provisions and scales an edge point-of-presence (PoP) in the FoundryDB edge tier. A PoP is one or more edge VMs in a single zone; a `target_node_count` of 2 or more runs a primary that holds the serving floating IP plus one or more hot standbys. The edge state machine converges the live `node_count` toward `target_node_count` asynchronously. Changing `target_node_count` scales the PoP in place; changing `zone` or `plan` destroys and recreates it. This resource covers the declarative edge fleet surface only; the imperative roll operation and the read-only fleet overview and recovery views are handled through the SDK and CLI.

#### Example

```hcl
resource "foundrydb_edge_node" "sto" {
  zone              = "se-sto1"
  target_node_count = 2
}
```

#### Arguments

| Argument | Type | Required | Forces Replace | Description |
|----------|------|----------|----------------|-------------|
| `zone` | string | Yes | Yes | Cloud zone the PoP is provisioned in (e.g. `se-sto1`). |
| `plan` | string | No | Yes | Compute plan for the PoP's edge VMs. When omitted, the platform selects the default edge plan. |
| `target_node_count` | number | No | No | Desired number of edge VMs in the PoP. `1` is a single-node PoP; `2` or more provisions a primary plus hot standbys. Changing this value scales the PoP in place. Default: `2`. |

#### Computed Attributes

| Attribute | Description |
|-----------|-------------|
| `id` | UUID of the edge PoP. |
| `name` | Platform-assigned name of the PoP. |
| `status` | Current lifecycle status: e.g. `provisioning`, `running`, `scaling`, `failed`. |
| `node_count` | Number of edge VMs currently live in the PoP; converges toward `target_node_count`. |

#### In-place Updates

Changing `target_node_count` scales the PoP without destroying it. All other arguments are immutable; changing `zone` or `plan` destroys and recreates the PoP.

---

### `foundrydb_stack`

Launches and manages a FoundryDB vertical-starter stack. A stack provisions a set of platform primitives (database, file storage, inference, app service) from a first-party catalog template or a customer-authored marketplace template in a single atomic operation. After creation, the provider waits up to 20 minutes for the stack to reach `Running` status.

All input fields are immutable after launch; any change destroys and recreates the stack. Use the `foundrydb_stack_templates` data source to discover available first-party templates and current prices.

#### Example (first-party template)

```hcl
data "foundrydb_stack_templates" "catalog" {}

locals {
  rag_template = one([
    for t in data.foundrydb_stack_templates.catalog.templates :
    t if t.name == "rag-chatbot"
  ])
}

resource "foundrydb_stack" "rag" {
  name                  = "prod-rag"
  template_name         = "rag-chatbot"
  accepted_monthly_cost = local.rag_template.monthly_total
}
```

#### Example (marketplace template)

```hcl
resource "foundrydb_stack" "analytics" {
  name                  = "team-analytics"
  template_id           = foundrydb_stack_template.custom.id
  accepted_monthly_cost = 50.00
}
```

#### Arguments

| Argument | Type | Required | Forces Replace | Description |
|----------|------|----------|----------------|-------------|
| `name` | string | Yes | Yes | Human-readable name for this stack instance. |
| `template_name` | string | No | Yes | First-party catalog template to launch (e.g. `rag-chatbot`). Use `foundrydb_stack_templates` to list available names. Exactly one of `template_name` or `template_id` must be set. |
| `template_id` | string | No | Yes | UUID of a customer-authored marketplace template to launch. Manage the template with `foundrydb_stack_template`. Exactly one of `template_name` or `template_id` must be set. |
| `organization_id` | string | No | Yes | Organization UUID to scope the stack and its inference key to. Defaults to the caller's primary billing organization. |
| `accepted_monthly_cost` | number | Yes | Yes | Estimated monthly cost in USD that the operator explicitly accepted. Read `monthly_total` from `foundrydb_stack_templates` (or from `fdb stack preview --template-id <id>` for marketplace templates) and pass it here. Launch is rejected if the platform's fresh estimate differs by more than $0.01. |

#### Computed Attributes

| Attribute | Description |
|-----------|-------------|
| `id` | UUID of the stack. |
| `status` | Current lifecycle status (e.g. `Running`, `Provisioning`, `Failed`). |
| `endpoint_url` | Public URL of the stack's primary application endpoint. Populated once `Running`. |
| `estimated_monthly_cost` | Actual estimated monthly cost in USD as recorded at launch time. |
| `resources` | List of child resources. Each item exposes `symbolic_name`, `kind`, `status`, and `service_id`. |

---

### `foundrydb_stack_template`

Creates and manages a customer-authored stack template in the FoundryDB marketplace. A template holds a `StackDescriptor` (JSON) that declares which platform primitives to compose (databases, file services, app services, inference keys) and how they depend on each other. Templates start in `draft` status and are only visible to the owning organization. Set `visibility` to `org_shared` or `public` and set `publish = true` to share the template.

Published templates are immutable on the platform: editing a published template requires unpublishing it first (via the API or CLI) or creating a new version. This provider allows updating `display_name`, `description`, `version`, `visibility`, and `descriptor` while the template is in `draft`, `rejected`, or `unpublished` state. The `name` argument forces resource replacement.

#### Example

```hcl
resource "foundrydb_stack_template" "analytics" {
  name         = "custom-analytics-stack"
  display_name = "Custom Analytics Stack"
  description  = "PostgreSQL + analytics app service for internal dashboards."
  version      = "1.0.0"
  visibility   = "org_shared"

  descriptor = jsonencode({
    apiVersion  = "stacks/v1"
    name        = "custom-analytics-stack"
    displayName = "Custom Analytics Stack"
    version     = "1.0.0"
    resources = [
      {
        name = "db"
        kind = "database"
        spec = {
          database_type   = "postgresql"
          version         = "17"
          plan_name       = "tier-2"
          zone            = "se-sto1"
          storage_size_gb = 50
          storage_tier    = "maxiops"
        }
      }
    ]
  })

  publish = true
}

output "template_id" {
  value = foundrydb_stack_template.analytics.id
}

output "publication_status" {
  value = foundrydb_stack_template.analytics.publication_status
}
```

#### Arguments

| Argument | Type | Required | Forces Replace | Description |
|----------|------|----------|----------------|-------------|
| `name` | string | Yes | Yes | Unique slug-style identifier for the template (e.g. `my-rag-stack`). Must be unique within the owning organization. |
| `display_name` | string | No | No | Human-readable name shown in the marketplace catalog. |
| `description` | string | No | No | Short description of what this template provisions. |
| `version` | string | No | No | Semantic version of the descriptor (e.g. `1.0.0`). Defaults to `1.0.0`. |
| `visibility` | string | No | No | Sharing scope: `private` (owning org only), `org_shared` (all org members), or `public` (marketplace, pending review). Default: `private`. |
| `descriptor` | string | Yes | No | JSON-encoded `StackDescriptor` validated by the platform. Use `jsonencode()` to compose it from a Terraform map or supply a raw string from `file()`. |
| `publish` | bool | No | No | When `true`, calls `PublishStackTemplate` after create (or after the update that first toggles this to `true`). For `org_shared`, publishes immediately; for `public`, submits to the moderation queue. Default: `false`. |

#### Computed Attributes

| Attribute | Description |
|-----------|-------------|
| `id` | UUID of the custom template. |
| `publication_status` | Moderation lifecycle: `draft`, `submitted`, `approved`, `published`, `rejected`, or `unpublished`. |
| `organization_id` | UUID of the organization that owns this template. |

#### Import

```bash
terraform import foundrydb_stack_template.analytics <template-uuid>
```

---

## Data Sources

### `foundrydb_database_user`

Retrieves the credentials for a database user. The `password` and `connection_string` attributes are marked sensitive.

#### Example

```hcl
data "foundrydb_database_user" "app" {
  service_id = foundrydb_service.postgres.id
  username   = "admin"
}

output "connection_string" {
  value     = data.foundrydb_database_user.app.connection_string
  sensitive = true
}
```

#### Arguments

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| `service_id` | string | Yes | UUID of the managed service. |
| `username` | string | Yes | Database username to retrieve credentials for. |

#### Computed Attributes

| Attribute | Sensitive | Description |
|-----------|-----------|-------------|
| `password` | Yes | Database user password. |
| `host` | No | Hostname to connect to. |
| `port` | No | Port number. |
| `database` | No | Default database name. |
| `connection_string` | Yes | Full connection string for a database driver. |

### `foundrydb_organizations`

Lists all organizations the authenticated user belongs to. Use the returned `id` values as the `organization_id` argument on `foundrydb_service` resources to scope them to a specific organization.

#### Example

```hcl
data "foundrydb_organizations" "all" {}

output "org_ids" {
  value = { for o in data.foundrydb_organizations.all.organizations : o.name => o.id }
}
```

#### Computed Attributes

The data source exposes an `organizations` list. Each entry contains:

| Attribute | Description |
|-----------|-------------|
| `id` | Unique identifier of the organization. |
| `name` | Display name. |
| `slug` | URL-friendly slug. |
| `role` | The authenticated user's role (e.g. `owner`, `member`). |
| `created_at` | RFC3339 creation timestamp. |

### `foundrydb_stack_templates`

Lists all available first-party stack templates from the FoundryDB catalog. Each template includes a fresh cost preview. Use the returned `name` and `monthly_total` values to populate a `foundrydb_stack` resource.

#### Example

```hcl
data "foundrydb_stack_templates" "catalog" {}

output "template_costs" {
  description = "Available stack templates and their estimated monthly costs"
  value = {
    for t in data.foundrydb_stack_templates.catalog.templates :
    t.name => t.monthly_total
  }
}
```

#### Computed Attributes

The data source exposes a `templates` list. Each entry contains:

| Attribute | Description |
|-----------|-------------|
| `name` | Machine-readable template name (e.g. `rag-chatbot`). Pass to `foundrydb_stack.template_name`. |
| `display_name` | Human-readable template name (e.g. `Launch a RAG chatbot`). |
| `description` | Short description of what this template provisions. |
| `version` | Semantic version of the template descriptor. |
| `monthly_total` | Estimated total monthly cost in USD. Pass to `foundrydb_stack.accepted_monthly_cost`. |

## Complete Examples

### PostgreSQL

```hcl
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

resource "foundrydb_service" "postgres" {
  name            = "prod-pg"
  database_type   = "postgresql"
  version         = "17"
  plan_name       = "tier-2"
  zone            = "se-sto1"
  storage_size_gb = 50
  storage_tier    = "maxiops"
  allowed_cidrs   = ["0.0.0.0/0"]
}

data "foundrydb_database_user" "app" {
  service_id = foundrydb_service.postgres.id
  username   = "admin"
}

output "service_id" {
  value = foundrydb_service.postgres.id
}

output "connection_string" {
  value     = data.foundrydb_database_user.app.connection_string
  sensitive = true
}
```

### MySQL

```hcl
resource "foundrydb_service" "mysql" {
  name            = "prod-mysql"
  database_type   = "mysql"
  version         = "8.4"
  plan_name       = "tier-3"
  zone            = "se-sto1"
  storage_size_gb = 100
  storage_tier    = "maxiops"
  allowed_cidrs   = ["10.0.0.0/8"]
}
```

### MongoDB

```hcl
resource "foundrydb_service" "mongo" {
  name            = "prod-mongo"
  database_type   = "mongodb"
  version         = "8.0"
  plan_name       = "tier-4"
  zone            = "se-sto1"
  storage_size_gb = 200
  storage_tier    = "maxiops"
  allowed_cidrs   = ["10.0.0.0/8"]
}
```

### Valkey

```hcl
resource "foundrydb_service" "valkey" {
  name            = "prod-cache"
  database_type   = "valkey"
  version         = "8.1"
  plan_name       = "tier-2"
  zone            = "se-sto1"
  storage_size_gb = 20
  storage_tier    = "maxiops"
  allowed_cidrs   = ["10.0.0.0/8"]
}
```

### Kafka

```hcl
resource "foundrydb_service" "kafka" {
  name            = "prod-kafka"
  database_type   = "kafka"
  version         = "3.9"
  plan_name       = "tier-4"
  zone            = "se-sto1"
  storage_size_gb = 500
  storage_tier    = "maxiops"
  allowed_cidrs   = ["10.0.0.0/8"]
}
```

### OpenSearch

```hcl
resource "foundrydb_service" "opensearch" {
  name            = "prod-opensearch"
  database_type   = "opensearch"
  version         = "2"
  plan_name       = "tier-4"
  zone            = "se-sto1"
  storage_size_gb = 200
  storage_tier    = "maxiops"
  allowed_cidrs   = ["10.0.0.0/8"]
}
```

### MSSQL

```hcl
resource "foundrydb_service" "mssql" {
  name            = "prod-mssql"
  database_type   = "mssql"
  version         = "4.8"
  plan_name       = "tier-4"
  zone            = "se-sto1"
  storage_size_gb = 100
  storage_tier    = "maxiops"
  allowed_cidrs   = ["10.0.0.0/8"]
}
```

### Organization-Scoped Service

Use `foundrydb_organizations` to look up available org IDs and scope a service to a specific organization:

```hcl
data "foundrydb_organizations" "all" {}

resource "foundrydb_service" "org_postgres" {
  name            = "org-prod-pg"
  database_type   = "postgresql"
  version         = "17"
  plan_name       = "tier-2"
  zone            = "se-sto1"
  storage_size_gb = 50
  storage_tier    = "maxiops"
  allowed_cidrs   = ["0.0.0.0/0"]

  # All API calls for this resource will include X-Active-Org-ID header.
  organization_id = data.foundrydb_organizations.all.organizations[0].id
}
```

### App Job

```hcl
resource "foundrydb_service" "app" {
  name            = "prod-app"
  database_type   = "app"
  plan_name       = "tier-2"
  zone            = "se-sto1"
  storage_size_gb = 20
  storage_tier    = "maxiops"
  allowed_cidrs   = ["0.0.0.0/0"]
}

resource "foundrydb_app_job" "nightly_cleanup" {
  app_service_id = foundrydb_service.app.id
  name           = "nightly-cleanup"
  schedule_cron  = "0 2 * * *"
  timezone       = "Europe/Stockholm"
  enabled        = true
  image_ref      = "registry.example.com/tools:cleanup-v2"
  command        = ["/app/cleanup", "--older-than=30d"]
  env = {
    LOG_LEVEL = "info"
  }
  max_retries          = 2
  retry_backoff_seconds = 60
  max_runtime_seconds  = 1800
  concurrency_cap      = 1
}

output "cleanup_job_id" {
  value = foundrydb_app_job.nightly_cleanup.id
}
```

### Queue

```hcl
resource "foundrydb_service" "pg" {
  name            = "prod-pg"
  database_type   = "postgresql"
  version         = "17"
  plan_name       = "tier-2"
  zone            = "se-sto1"
  storage_size_gb = 50
  storage_tier    = "maxiops"
  allowed_cidrs   = ["0.0.0.0/0"]
}

resource "foundrydb_queue" "tasks" {
  service_id                 = foundrydb_service.pg.id
  name                       = "tasks"
  visibility_timeout_seconds = 30
  max_attempts               = 5
  dlq_enabled                = true
}

output "tasks_queue_status" {
  value = foundrydb_queue.tasks.status
}

output "tasks_queue_database" {
  value = foundrydb_queue.tasks.database_name
}
```

### Stack (RAG chatbot, first-party template)

```hcl
# Discover templates and their current prices.
data "foundrydb_stack_templates" "catalog" {}

locals {
  rag_template = one([
    for t in data.foundrydb_stack_templates.catalog.templates :
    t if t.name == "rag-chatbot"
  ])
}

# Launch the stack. accepted_monthly_cost is read from the catalog so the
# configuration stays in sync with platform pricing automatically.
resource "foundrydb_stack" "rag" {
  name                  = "prod-rag"
  template_name         = "rag-chatbot"
  accepted_monthly_cost = local.rag_template.monthly_total
}

output "rag_endpoint" {
  description = "Chat UI URL (available once Running)"
  value       = foundrydb_stack.rag.endpoint_url
}

output "rag_resources" {
  description = "Child platform resources provisioned by the stack"
  value       = foundrydb_stack.rag.resources
}
```

### Stack (marketplace custom template)

```hcl
# Author and publish a custom template, then launch a stack from it.
resource "foundrydb_stack_template" "analytics" {
  name         = "custom-analytics-stack"
  display_name = "Custom Analytics Stack"
  description  = "PostgreSQL + analytics app for internal dashboards."
  version      = "1.0.0"
  visibility   = "org_shared"

  descriptor = jsonencode({
    apiVersion  = "stacks/v1"
    name        = "custom-analytics-stack"
    displayName = "Custom Analytics Stack"
    version     = "1.0.0"
    resources = [
      {
        name = "db"
        kind = "database"
        spec = {
          database_type   = "postgresql"
          version         = "17"
          plan_name       = "tier-2"
          zone            = "se-sto1"
          storage_size_gb = 50
          storage_tier    = "maxiops"
        }
      }
    ]
  })

  publish = true
}

resource "foundrydb_stack" "analytics" {
  name                  = "team-analytics"
  template_id           = foundrydb_stack_template.analytics.id
  accepted_monthly_cost = 50.00

  depends_on = [foundrydb_stack_template.analytics]
}

output "analytics_endpoint" {
  value = foundrydb_stack.analytics.endpoint_url
}
```

## Import

Existing services can be imported using their UUID:

```bash
terraform import foundrydb_service.postgres <service-uuid>
```

Existing stacks can be imported using their UUID:

```bash
terraform import foundrydb_stack.rag <stack-uuid>
```

Existing custom templates can be imported using their UUID:

```bash
terraform import foundrydb_stack_template.analytics <template-uuid>
```

## License

MIT. See [LICENSE](LICENSE).
