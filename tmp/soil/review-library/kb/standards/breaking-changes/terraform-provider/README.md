# Terraform Provider — Handling Breaking API Changes

## Overview

When an API object changes shape (e.g., a flat map becomes a nested block), the Terraform provider **must** include a state upgrader so that existing users' state files are migrated automatically. Without this, users hit errors on `terraform plan` / `terraform apply` and are forced to write manual migration scripts or taint and recreate resources — both unacceptable.

## What Requires a State Upgrader

Any change to how a field is represented in the Terraform schema that would cause existing state to be invalid:

- **Type changes** — e.g., `TypeMap` (flat `map(string)`) to `TypeList` / `TypeSet` with nested blocks
- **Nesting changes** — e.g., a JSON-encoded string field becoming a first-class nested `content {}` block
- **Field removals** — a field that existed in state but is no longer in the schema
- **Field renames** — a field that changes name between versions

## How State Upgraders Work

Terraform tracks a `SchemaVersion` integer on each resource. When the provider bumps this version, Terraform runs the registered upgrade functions sequentially to migrate the raw state from the old version to the new one.

### Pattern

```go
// 1. Define the upgrade function
func upgradeResourceStateV0ToV1(ctx context.Context, rawState map[string]any, meta any) (map[string]any, error) {
    // Transform old flat map to new nested block structure.
    // Old state: "alert_payload_conditions" => map[string]string{"logic": "ALL", "conditions": "<json>"}
    // New state: "alert_payload_conditions" => []map[string]interface{}{{"logic": "ALL", "conditions": [...]}}
    if old, ok := rawState["alert_payload_conditions"]; ok {
        oldMap, _ := old.(map[string]interface{})
        if oldMap != nil {
            newBlock := map[string]interface{}{
                "logic": oldMap["logic"],
            }
            if raw, ok := oldMap["conditions"].(string); ok {
                var parsed []interface{}
                if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
                    newBlock["conditions"] = parsed
                }
            }
            rawState["alert_payload_conditions"] = []interface{}{newBlock}
        }
    }
    return rawState, nil
}

// 2. Define the old schema (only fields that changed)
func resourceExampleV0() *schema.Resource {
    return &schema.Resource{
        Schema: map[string]*schema.Schema{
            "alert_payload_conditions": {
                Type:     schema.TypeMap,
                Optional: true,
                Elem: &schema.Schema{
                    Type: schema.TypeString,
                },
            },
        },
    }
}

// 3. Wire it up on the resource
func resourceExample() *schema.Resource {
    return &schema.Resource{
        // ... CRUD functions ...
        SchemaVersion: 1, // bump this
        StateUpgraders: []schema.StateUpgrader{
            {
                Version: 0,
                Type:    resourceExampleV0().CoreConfigSchema().ImpliedType(),
                Upgrade: upgradeResourceStateV0ToV1,
            },
        },
        Schema: map[string]*schema.Schema{
            // ... current schema with new nested block ...
        },
    }
}
```

### Real Example: Schedule Resource

In `provider/resource_schedule.go`, the `slack_user_group` field was removed from the schema. A state upgrader was added to clean it from existing state:

```go
func upgradeScheduleStateV0ToV1(ctx context.Context, rawState map[string]any, meta any) (map[string]any, error) {
    delete(rawState, "slack_user_group")
    return rawState, nil
}
```

## Checklist

Before merging any PR that changes a Terraform resource schema:

- [ ] Does this change alter the type, nesting, or name of an existing field?
- [ ] If yes — have you bumped `SchemaVersion`?
- [ ] If yes — have you added a `StateUpgrader` that converts old state to the new shape?
- [ ] Have you tested the upgrade path with a real state file from the previous version?
- [ ] Is the change documented in the CHANGELOG with a migration note?

## What Happens If You Skip This

If a breaking schema change ships without a state upgrader, existing users will see errors on `terraform plan` / `terraform apply` such as:

- **Type mismatch errors** — e.g., `Incorrect attribute value type: inappropriate value for attribute "X": object required` when state holds a flat map but the schema now expects a nested block
- **Unexpected destroy-and-recreate plans** — Terraform sees the old state as incompatible and proposes destroying the resource and creating a new one, causing downtime
- **Inconsistent state after apply** — The resource appears to update successfully but subsequent plans keep showing diffs because state never converges to the new shape

The only workarounds for users are:

1. Manually editing `.tfstate` (dangerous, error-prone)
2. Writing a migration script (burden on the customer)
3. Tainting and recreating the resource (causes downtime)

None of these are acceptable. The provider must handle migration automatically.
