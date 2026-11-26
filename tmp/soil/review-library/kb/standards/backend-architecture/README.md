# Backend Architecture

## Thin Controllers

Controllers should coordinate the request/response cycle, not implement business logic. Keep them thin.

- **Delegate business logic** to services (see [Service Pattern](../rails/service-pattern.md)) or models. Controllers should orchestrate, not decide.
- **Keep actions short.** An action should authenticate, authorize, dispatch to a service or model, and render a response — little more.
- **Stick to RESTful actions** (`index`, `show`, `new`, `create`, `edit`, `update`, `destroy`). Reach for a new controller before adding non-standard actions.
- **Push data access into models or [query objects](https://thoughtbot.com/blog/a-case-for-query-objects-in-rails)** — plain Ruby classes that encapsulate a single complex query. Avoid building complex queries inline in the controller.
- **Handle only HTTP concerns** in the controller: params, sessions, cookies, status codes, redirects, and rendering.
- **Avoid callbacks for business logic.** Use `before_action` for request setup (loading records, authorization), not for domain rules.
- **Return early** on authorization failures and on unsuccessful service/model results to keep the happy path readable.

## Rich Domain Models, Organized by Capability

Keep domain behavior close to the model that owns its state. Controllers and
jobs should call clear model APIs such as `incident.schedule_meeting`, not
coordinate that capability's rules themselves.

Large, central models MAY group one cohesive capability in a nested,
model-private concern. For example, `Incident::Meetings` can own
meeting-specific associations, scopes, callbacks, validations, and methods
while `Incident` remains the public API. This is organization, not an excuse
to move unrelated logic out of sight.

Use a separate domain object when an operation has its own lifecycle, spans
multiple aggregates, or coordinates external systems. The public operation
should still have a natural home on the owning domain model when one exists.

See [Domain-Focused Model Concerns](../rails/model-concerns.md) for the Rails
file/constant convention, extraction criteria, and review checklist.