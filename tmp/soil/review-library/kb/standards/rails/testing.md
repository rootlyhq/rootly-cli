# RSpec Best Practices

> **Note:** These are aspirational best practices we are working towards. Our current codebase does not fully follow these patterns yet. Treat these as **highly encouraged guidelines**, not strict requirements. When writing new tests or refactoring existing ones, aim to follow these practices - but don't feel obligated to refactor unrelated code to match.

Guidelines for writing fast, maintainable RSpec tests at Rootly.

## Resources

- [TestProf Documentation](https://test-prof.evilmartians.io/) - Ruby tests profiling and optimization toolbox
- [Better Specs](https://www.betterspecs.org/) - RSpec guidelines
- [Software Unit Test Smells](https://testsmells.org/) - Testing anti-patterns

## General Principles

### Keep Assertions Focused

Each test should verify one behavior. A small number of assertions (under 5) per test is fine when they all relate to the same behavior. This makes failures easier to diagnose and tests easier to maintain.

Request specs often benefit from more assertions as part of a single flow (e.g., checking status code, response body, and side effects together).

```ruby
# ✅ Good: Single focused expectation
it "returns the user's full name" do
  user = build(:user, first_name: "Jane", last_name: "Doe")
  expect(user.full_name).to eq("Jane Doe")
end

# ✅ Good: Multiple related assertions about the same behavior
it "creates the incident" do
  expect(response).to have_http_status(:created)
  expect(response.parsed_body["title"]).to eq("Outage")
  expect(Incident.count).to eq(1)
end

# ❌ Bad: Many unrelated assertions testing different behaviors
it "handles names correctly" do
  user = build(:user, first_name: "Jane", last_name: "Doe")
  expect(user.full_name).to eq("Jane Doe")
  expect(user.initials).to eq("JD")
  expect(user.display_name).to eq("Jane D.")
  expect(user.email).to include("@")
  expect(user).to be_valid
end
```

### Avoid Deeply Nested Contexts

Keep context nesting shallow (max 2-3 levels). When a reader must trace through multiple nested `before` blocks and `let` declarations to understand what's being tested, the test becomes hard to maintain.

**Note:** This doesn't conflict with using `let_it_be` for shared data (see [TestProf Optimization](#testprof-optimization)). The problem is scattered, nested setup that modifies state—not shared read-only data declared at the top of a describe block.

```ruby
# ✅ Good: Setup is close to the assertion
describe "#resolve!" do
  it "marks the incident as resolved" do
    team = create(:team)
    user = create(:user, current_team: team, teams: [team])
    incident = create(:incident, team: team, user: user, status: :started)

    incident.resolve!

    expect(incident.reload.status).to eq("resolved")
  end
end

# ❌ Bad: Setup scattered across nested contexts
describe Incident do
  let(:team) { create(:team) }
  let(:user) { create(:user, current_team: team, teams: [team]) }
  let(:incident) { create(:incident, team: team, user: user) }

  context "when started" do
    before { incident.update(status: :started) }

    context "with severity assigned" do
      before { incident.update(severity: create(:severity, team: team)) }

      describe "#resolve!" do
        it "marks as resolved" do
          # Reader must trace through multiple levels to understand setup
          incident.resolve!
          expect(incident.reload.status).to eq("resolved")
        end
      end
    end
  end
end
```

### Name Your Subject

Avoid implicit subjects. Named subjects improve readability.

```ruby
# ✅ Good: Named subject
describe Incidents::Resolver do
  subject(:resolver) { described_class.new(incident, resolved_by: user) }

  let(:incident) { create(:incident, status: :started) }
  let(:user) { create(:user) }

  it "marks incident as resolved" do
    resolver.call
    expect(incident.reload.status).to eq("resolved")
  end
end

# ❌ Bad: Implicit subject
describe Incidents::Resolver do
  subject { described_class.new(incident, resolved_by: user) }

  it "marks incident as resolved" do
    subject.call  # What is "subject"?
    expect(incident.reload.status).to eq("resolved")
  end
end
```

### Test Behavior, Not Implementation

Focus on what the code does, not how it does it.

```ruby
# ✅ Good: Tests the outcome
it "notifies on-call users when alert is triggered" do
  team = create(:team)
  schedule = create(:schedule, team: team)
  alert = create(:alert, team: team, status: :triggered)

  expect(alert.notified_users).to include(schedule.current_on_call_user)
end

# ❌ Bad: Tests internal implementation when testing Alert behavior
# If you're testing Alert#trigger!, verify the outcome, not the implementation
it "calls NotificationService for each on-call user" do
  team = create(:team)
  alert = create(:alert, team: team)

  expect(Alerts::NotificationService).to receive(:notify).exactly(3).times

  alert.trigger!
end
# Note: Mocking collaborators IS appropriate when testing the service itself
# or when you need to isolate a unit from external dependencies
```

## Factory Best Practices

### Prefer build_stubbed Over create

`build_stubbed` is fastest, `build` is fast, `create` is slow. Only use `create` when you need database persistence.

```ruby
# ✅ Good: Use build_stubbed for unit tests
describe User do
  it "validates email format" do
    user = build_stubbed(:user, email: "invalid")
    expect(user).not_to be_valid
  end
end

# ✅ Good: Use build when you need callbacks but not persistence
describe Incident do
  it "generates a slug before validation" do
    incident = build(:incident, title: "Database Outage")
    incident.valid?
    expect(incident.slug).to be_present
  end
end

# ✅ Good: Use create only when persistence is required
describe "GET /api/v1/incidents/:id" do
  it "returns the incident" do
    incident = create(:incident)
    get "/api/v1/incidents/#{incident.id}"
    expect(response.parsed_body["data"]["id"]).to eq(incident.id.to_s)
  end
end
```

### Prefer Factories Without Side Effects

Factories should ideally not trigger expensive callbacks (Slack notifications, AI analysis, etc.). When factories must have callbacks, use explicit opt-in traits to enable them only when needed.

```ruby
# ✅ Best: Factory produces minimal object by default
factory :alert do
  summary { "Test Alert" }
  # No automatic notifications, integrations, etc.
end

# ✅ Good: Use :minimal trait when available
describe Alert do
  it "validates summary presence" do
    alert = build(:alert, :minimal, summary: nil)
    expect(alert).not_to be_valid
  end
end

# ✅ Good: Explicitly opt-in to callbacks only when testing them
describe "alert notifications" do
  it "announces on Slack when triggered" do
    alert = create(:alert, :with_notifications)
    expect(alert).to have_announced_on_slack
  end
end
```

### Avoid Factory Cascades

Factory cascades occur when factories create associated records that create more associated records. This is the #1 cause of slow tests.

```ruby
# ❌ Bad: Factory cascade - creates team, user, severity, services for every test
FactoryBot.define do
  factory :incident do
    title { "Server Down" }
    team
    user
    severity
    association :created_by, factory: :user
  end
end

# Running create(:incident) might create 10+ database records

# ✅ Good: Use transient attributes and explicit associations
FactoryBot.define do
  factory :incident do
    title { Faker::Lorem.sentence }
    started_at { Time.zone.now }
    severity { nil }

    trait :with_severity do
      severity
    end

    trait :with_services do
      transient { services_count { 1 } }
      after(:create) do |incident, evaluator|
        create_list(:incidents_service, evaluator.services_count, incident: incident)
      end
    end
  end
end

# Tests explicitly declare what they need
create(:incident, team: team, user: user, severity: severity)
create(:incident, :with_services, services_count: 3)
```

### Avoid Random Data Where Uniqueness Matters

Faker can cause flaky tests due to random unique constraint violations.

```ruby
# ❌ Bad: Random data can collide in parallel tests
factory :user do
  email { Faker::Internet.email }
end

# ✅ Good: Use sequences for unique fields
factory :user do
  sequence(:email) { |n| "user#{n}@example.com" }
end

factory :team do
  sequence(:name) { |n| "Team #{n}" }
  sequence(:slug) { |n| "team_#{n}" }
end
```

## TestProf Optimization

[TestProf](https://test-prof.evilmartians.io/) provides tools to profile and speed up your test suite.

### Profiling Your Tests

Identify slow factories with FactoryProf:

```bash
# See factory usage statistics
FPROF=1 bundle exec rspec

# Generate flamegraph to visualize factory cascades
FPROF=flamegraph bundle exec rspec
```

If your flamegraph looks like a city skyline, you have factory cascade problems.

### let_it_be

Use `let_it_be` instead of `let` when the record doesn't need to change between examples. It creates the record once per describe block instead of once per example.

```ruby
# ✅ Good: Record created once, reused across examples
describe ShiftPolicy do
  let_it_be(:team) { create(:team) }
  let_it_be(:user) { create(:user, current_team: team, teams: [team]) }
  let_it_be(:schedule) { create(:schedule, team: team, created_by_user: user, skip_shift_creation: true) }
  let_it_be(:shift) { create(:shift, schedule: schedule, user: user) }

  it "allows team members to view shifts" do
    expect(described_class.new(user, shift).view?).to be true
  end

  it "allows team members to swap shifts" do
    expect(described_class.new(user, shift).swap?).to be true
  end
end

# ❌ Bad: Record recreated for every single example
describe ShiftPolicy do
  let(:team) { create(:team) }
  let(:user) { create(:user, current_team: team, teams: [team]) }
  let(:schedule) { create(:schedule, team: team, created_by_user: user) }
  let(:shift) { create(:shift, schedule: schedule, user: user) }

  # Same tests, but 4x slower due to repeated record creation
end
```

**Caveats:**
- Don't use `let_it_be` in migration specs - they can't run inside transactions
- Don't mutate `let_it_be` records in tests - they're shared across examples

### before_all

Use `before_all` for complex setup that goes beyond simple record creation (e.g., API calls, file setup). For creating records, prefer `let_it_be` instead.

```ruby
# ✅ Prefer let_it_be for record creation
describe "Escalation policy notifications" do
  let_it_be(:team) { create(:team) }
  let_it_be(:escalation_policy) { create(:escalation_policy, :with_default_path, team: team) }
  let_it_be(:users) { create_list(:user, 3, current_team: team, teams: [team]) }

  it "notifies first level users" do
    # tests using team, escalation_policy, users
  end
end

# ✅ Use before_all for non-record setup
describe "Report export" do
  before_all do
    @export_dir = Rails.root.join("tmp", "test_exports")
    FileUtils.mkdir_p(@export_dir)
  end

  after_all do
    FileUtils.rm_rf(@export_dir)
  end

  it "writes CSV to export directory" do
    # test using @export_dir
  end
end
```

### create_default (FactoryDefault)

> **Aspirational:** This is a TestProf feature we're evaluating for future use. Not currently used in the codebase.

Use `create_default` to eliminate cascades by reusing associated records. Best for top-level entities like teams.

```ruby
describe "Incident management" do
  let_it_be(:team) { create_default(:team) }
  let_it_be(:user) { create_default(:user, current_team: team, teams: [team]) }

  it "creates incident in default team" do
    # FactoryBot automatically uses the default team
    incident = create(:incident)
    expect(incident.team).to eq(team)
  end

  it "assigns to default user" do
    incident = create(:incident)
    expect(incident.user).to eq(user)
  end
end
```

**Use sparingly:** Only use `create_default` for top-level associations. Keep default declarations at the top of test files for visibility.

### AnyFixture

> **Aspirational:** This is a TestProf feature we're evaluating for future use. Not currently used in the codebase. Requires careful planning as it adds overhead for single test runs.

Use `AnyFixture` for data that should exist for the entire test run (like seed data).

```ruby
# spec/support/any_fixtures.rb
RSpec.configure do |config|
  config.before(:suite) do
    TestProf::AnyFixture.register(:default_severities) do
      team = Team.first || create(:team)
      Severity.create!(name: "Critical", team: team, severity: :critical)
      Severity.create!(name: "High", team: team, severity: :high)
      Severity.create!(name: "Low", team: team, severity: :low)
    end
  end
end
```

## External API Requests

### Use VCR for HTTP Interactions

[VCR](https://github.com/vcr/vcr) records real HTTP interactions and replays them in tests. This gives us tests that validate against actual API response shapes instead of hand-crafted mocks.

```ruby
# ✅ Good: VCR cassette via metadata tag
it "returns embeddings", vcr: "open_ai_client/embedding" do
  result = client.embedding(model: "text-embedding-3-small", input: "hello world")
  expect(result.size).to eq(1536)
end

# ✅ Good: Explicit VCR.use_cassette
it "fetches PagerDuty teams" do
  VCR.use_cassette("pagerduty/list_teams") do
    teams = client.teams(account)
    expect(teams).to be_present
  end
end

# ❌ Bad: Hand-crafting Faraday::Response objects — response shape can drift from reality
let(:mock_response) do
  Faraday::Response.new(
    status: 200,
    body: [
      {
        name: "tokenName",
        expirationDuration: 90
      }.with_indifferent_access
    ],
  )
end

before do
  allow(subject).to receive(:create_client).and_return(mock_client)
  allow(mock_client).to receive(:get).and_return(mock_response)
end

# ❌ Bad: Mocking response doubles — same drift risk, plus hides real HTTP behavior
it "returns results" do
  mock_response = double(status: 200, body: {"results" => [{"id" => "block1"}]})
  allow(mock_client).to receive(:get).and_return(mock_response)
  allow(described_class).to receive(:handle_response).and_return(mock_response.body)
  # ...
end
```

### When to use VCR vs WebMock

**VCR cassettes** — for tests where you want to verify the client correctly handles a real API response.

**WebMock stubs** — for tests that need to assert what was _sent_ in the request body (e.g., parameter transformation, PII scrubbing):

```ruby
# ✅ WebMock is appropriate here — we're testing what we send, not what we receive
it "converts max_tokens to max_completion_tokens" do
  stub_request(:post, "https://api.openai.com/v1/chat/completions")
    .with { |req| JSON.parse(req.body)["max_completion_tokens"] == 1000 }
    .to_return(status: 200, body: { ... }.to_json, headers: {"Content-Type" => "application/json"})

  client.raw_chat_completion(max_tokens: 1000, ...)
end
```

VCR and WebMock coexist — WebMock stubs take precedence over VCR cassettes when both are active.


## Common Patterns

### Testing with Feature Flags (Flipper)

```ruby
describe Incident do
  let(:team) { create(:team) }
  let(:incident) { create(:incident, team: team) }

  context "when ai_summary feature is enabled" do
    before do
      stub_flipper(name: :ai_summary, team: team, value: true)
    end

    it "generates AI summary" do
      expect(incident.ai_summary).to be_present
    end
  end

  context "when ai_summary feature is disabled" do
    before do
      stub_flipper(name: :ai_summary, team: team, value: false)
    end

    it "returns nil for AI summary" do
      expect(incident.ai_summary).to be_nil
    end
  end
end
```

### Testing Time-Dependent Logic

```ruby
describe Schedule do
  let_it_be(:team) { create(:team) }
  let_it_be(:schedule) { create(:schedule, team: team) }

  describe "#current_on_call_user" do
    it "returns the user on-call at the given time" do
      travel_to(Time.zone.parse("2024-01-15 10:00:00")) do
        user = create(:user, current_team: team, teams: [team])
        create(:shift, schedule: schedule, user: user,
               starts_at: 1.hour.ago, ends_at: 1.hour.from_now)

        expect(schedule.current_on_call_user).to eq(user)
      end
    end
  end
end
```

### Testing Multi-Team Scenarios

```ruby
describe User do
  describe "#slack_account" do
    let_it_be(:team_a) { create(:team) }
    let_it_be(:team_b) { create(:team) }
    let_it_be(:user) { create(:user, skip_set_default_notification_rule: true) }

    context "when team has no slack integration" do
      it "returns nil" do
        expect(user.slack_account(team_a)).to be_nil
      end
    end

    context "when user has oauth accounts for multiple teams" do
      let!(:team_a_slack) { create(:integrations_slack_account, team: team_a) }
      let!(:team_a_oauth) { create(:oauth_slack_account, user: user, team: team_a) }

      it "returns the oauth account for the specified team" do
        expect(user.slack_account(team_a)).to eq(team_a_oauth)
      end
    end
  end
end
```

## Anti-Patterns to Avoid

### Mystery Guest

Don't rely on data created far from the test.

```ruby
# ❌ Bad: Where do these incidents come from?
it "shows user incidents" do
  get "/api/v1/incidents"
  expect(response.parsed_body["data"].size).to eq(3)
end

# ✅ Good: Setup is explicit
it "shows user incidents" do
  team = create(:team)
  user = create(:user, current_team: team, teams: [team])
  create_list(:incident, 3, team: team, user: user)

  sign_in(user)
  get "/api/v1/incidents"

  expect(response.parsed_body["data"].size).to eq(3)
end
```

### Shared State Between Examples

Each example should be independent. Don't rely on execution order. Consider using [Isolator](https://github.com/palkan/isolator) to detect and prevent accidental cross-test dependencies.

```ruby
# ❌ Bad: Examples depend on each other
describe Incident do
  it "creates an incident" do
    @incident = create(:incident)
    expect(@incident).to be_persisted
  end

  it "can be resolved" do
    @incident.resolve!  # Fails if run alone
    expect(@incident.status).to eq("resolved")
  end
end

# ✅ Good: Each example is self-contained
describe Incident do
  it "can be resolved" do
    incident = create(:incident, status: :started)
    incident.resolve!
    expect(incident.status).to eq("resolved")
  end
end
```

### Testing Private Methods

Test the public interface, not private implementation.

```ruby
# ❌ Bad: Testing private method directly
it "formats the slack message" do
  alert = build(:alert)
  expect(alert.send(:format_slack_message)).to include("Alert triggered")
end

# ✅ Good: Test through public interface
it "sends formatted notification to Slack" do
  alert = create(:alert, :full)
  expect(alert.slack_message).to include("Alert triggered")
end
```

### Avoid Controller Specs

Don't write controller specs. Rails has discouraged them since Rails 5.

Use request specs when you need to exercise the HTTP flow end-to-end: routing, middleware, authentication, controller, and response. This is the right default for "does this endpoint work".

The majority of tests should be written at a more granular level, e.g. model, service or component specs.

### Mocking Style: Classicist vs Mockist

There are two valid testing styles:
- **Classicist (Detroit)**: Test with real objects, verify outcomes
- **Mockist (London)**: Use test doubles, verify interactions

We generally prefer the **classicist approach** for its stronger guarantees, but mockist style is acceptable when isolating from slow or external dependencies.

```ruby
# Mockist approach: verifies the interaction
it "resolves incident" do
  incident = instance_double(Incident)
  allow(incident).to receive(:resolve!)

  Incidents::Resolver.new(incident).call

  expect(incident).to have_received(:resolve!)
end

# Classicist approach (preferred): verifies the outcome
it "resolves incident" do
  incident = create(:incident, status: :started)

  Incidents::Resolver.new(incident).call

  expect(incident.reload.status).to eq("resolved")
end
```

**Avoid over-mocking:** Don't stub methods you don't need. Excessive stubs create brittle tests that can pass even when code is broken.

## Measuring Test Performance

Run TestProf profilers regularly to catch performance regressions:

```bash
# Profile factory usage
FPROF=1 bundle exec rspec

# Profile by example group tags
TAG_PROF=type bundle exec rspec

# Analyze setup vs test time
RD_PROF=1 bundle exec rspec
```

Target: Tests should run at **100+ examples per minute** for a healthy suite.
