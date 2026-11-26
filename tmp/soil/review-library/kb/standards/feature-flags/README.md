# Feature Flags

At Rootly, feature flags go through **`FeatureFlag.enabled?`**, our facade over
LaunchDarkly (LD). All new flags must use this path.

> **Flipper is deprecated.** Do not add new `Flipper` flags — the
> `NoNewFlipperFlags` RuboCop cop fails CI on them. Migrating an in-flight
> Flipper PR? See the [Flipper → LD migration guide](#deprecated-flipper).

## How to add a feature flag

> Step-by-step with screenshots: [Creating a flag in LaunchDarkly](./creating-a-flag-in-launchdarkly.md).

1. Create the flag in **LaunchDarkly** with the required metadata (below).
2. Reference it in code with `FeatureFlag.enabled?("my-flag")`.
3. Roll out via LD (boolean, percentage, or segment).
4. Once fully rolled out, **remove the flag and the dead branch** it guarded.

### Required metadata at creation time

Every flag must have, set in LaunchDarkly:

- **Maintainer** — assign the owning person/team as the flag's Maintainer in LD.
  A flag with no owner never gets cleaned up.
- **Retirement date** — state the intended removal date in the flag's
  **description**. Flags are temporary by default.
- **Rollout strategy** — boolean, percentage, or segment, configured in LD.

## Naming conventions

Flag names are checked automatically at lint time (CI) and mirror the
LaunchDarkly-side rule configured via Terraform. A name must be:

- **kebab-case** — lowercase letters, digits, and single hyphens
  (`foo-bar-routing`). Because hyphens aren't valid in bare Ruby symbols, pass
  flag names as **string literals**, not symbols.
- **No `enable-` / `disable-` verb prefix** — the flag already implies a toggle,
  so name it for the feature, not the verb. Use `foo-bar-routing`, not
  `enable-foo-bar-routing`.
- **A literal string** — the argument to `FeatureFlag.enabled?` /
  `FeatureFlag.variation` must be a literal, never an interpolated or computed
  name. This keeps every flag greppable and lets later tooling analyze call sites.

```ruby
# Good — kebab-case, no verb prefix, literal name
FeatureFlag.enabled?("foo-bar-routing")
FeatureFlag.variation("foo-bar-routing", team:)

# Bad — enable-/disable- verb prefix
FeatureFlag.enabled?("enable-foo")
FeatureFlag.enabled?("disable-foo")

# Bad — not kebab-case
FeatureFlag.enabled?("DontDoBar")
FeatureFlag.enabled?("foo_bar_routing")

# Bad — dynamic / non-literal name (not greppable)
FeatureFlag.enabled?("#{feature}-routing")
flag = "foo-bar-routing"
FeatureFlag.enabled?(flag)
```

### Always go through the `FeatureFlag` facade

Reference flags via `FeatureFlag.enabled?` / `FeatureFlag.variation` — never the
LaunchDarkly SDK directly. Direct `LaunchDarkly::LDClient` / `LaunchDarkly::Config`
references in application code fail CI.

```ruby
# Good — global check
FeatureFlag.enabled?("foo-bar-routing")

# Good — scoped to a context (team, user, etc.)
FeatureFlag.enabled?("foo-bar-routing", team:)
FeatureFlag.enabled?("foo-bar-routing", user:)
FeatureFlag.variation("foo-bar-routing", team:)

# Bad — reaches for the LD SDK directly
LaunchDarkly::LDClient.new(...).variation("foo-bar-routing", context, false)
```

## When to use a feature flag

- **Gradual rollouts** — release to a small % of users to watch for issues.
- **Internal testing / dogfooding** — enable for internal users pre-launch.
- **Beta programs** — early access for opt-in customers.
- **Kill switches** — instantly disable a problematic feature without a deploy.
- **Operational controls** — toggle expensive or risky operations.
- **Infrastructure migrations** — switch between old and new infrastructure.
- **Long-running / incomplete features** — merge incomplete work safely behind a flag.

## When *not* to use a feature flag

- **Permanent configuration** — flags are temporary. Long-lived toggles belong
  as **team configurations**, not flags. (See the
  [Migrating Flipper → LD](https://app.notion.com/p/rootly/Migrating-Flipper-LD-3675a77a0aca80c2ab74ef748bab9836)
  initiative.)
- **No cleanup plan** — a flag that won't be removed becomes tech debt. Don't add
  one without a lifecycle plan and a retirement date.
- **To avoid proper versioning** — don't bypass API versioning, schema changes,
  or migrations with a flag.
- **Large structural changes** — flags work best on contained logic. Changes
  spanning DB + backend + frontend are hard to manage via a flag.
- **In shared libraries or SDKs** — flags injected into reusable packages are
  hard to manage and decouple.
- **When rollback is cleaner** — for isolated, easy-to-revert changes, a deploy
  rollback can beat managing a toggle.

## Best practices

### 1. Guard the code so the deprecated branch is obvious

Implement the flag so it's clear which code gets deleted once the flag is fully
on. Keep both branches in the same place — don't split the new path into a
separate file that the cleanup will miss.

```erb
<!-- app/views/v2/account/_form.html.erb -->
<% if FeatureFlag.enabled?("my-new-form-feature") %>
  <%= form_with(model: [:account, model]) do |form| %>
    <h6 class="element-header mt-4">My v2 Form</h6>
  <% end %>
<% else %>
  <%= form_with(model: [:account, model]) do |form| %>
    <h6 class="element-header mt-4">My v1 Form</h6>
  <% end %>
<% end %>
```

### 2. Don't assign flags to variables

Assigning a flag check to a variable makes cleanup harder — a grep for the flag
name no longer finds every use. Passing the flag *name* through a variable is
also rejected at lint time (see [Naming conventions](#naming-conventions)).

```ruby
# Good
def my_method(args)
  if FeatureFlag.enabled?("my-new-output")
    puts "new"
  else
    puts "old"
  end
end

# Bad
def my_method(args)
  is_new_output_enabled = FeatureFlag.enabled?("my-new-output")
  puts is_new_output_enabled ? "new" : "old"
end
```

### 3. Clean up flags promptly

Once a flag is fully rolled out (or retired), remove the `FeatureFlag.enabled?`
call and the dead branch, then archive the flag in LD. The retirement date in
the description is your reminder.

## <a name="deprecated-flipper"></a>Deprecated: Flipper

Flipper is no longer used for new flags. The `NoNewFlipperFlags` cop fails CI on
any new `Flipper` flag.

**In-flight Flipper PRs:** replace `Flipper.enabled?(:foo)` with
`FeatureFlag.enabled?("foo")` and create the flag in LaunchDarkly with the
required metadata above before merging.
