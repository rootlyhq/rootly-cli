# Testing Guidelines for Developers

## Overview

This document provides developers with guidelines for building testable features and supporting QA automation efforts at Rootly. Focus is on making features easy to test through proper markup and collaborative practices.

## Supporting E2E Test Automation

### Adding Test Attributes During Development

When building new features, include `data-testid` attributes to support future test automation:

```erb
<%# ✅ Good: Include test IDs during feature development %>
<%= form_with model: @user, local: true do |form| %>
  <%= form.email_field :email,
      data: { testid: 'registration-form-email-input' },
      placeholder: 'Enter your email',
      required: true %>

  <%= form.password_field :password,
      data: { testid: 'registration-form-password-input' },
      placeholder: 'Create password' %>

  <%= form.submit 'Create Account',
      data: { testid: 'registration-form-submit-button' },
      class: 'btn btn-primary' %>
<% end %>
```

### When to Add Test IDs

**Always include for:**
- Form inputs and submit buttons
- Navigation links and menu items
- Action buttons (edit, delete, save, cancel)
- Modal dialogs and their controls
- Data tables and their action buttons
- Status indicators and alerts

**Naming conventions:**
```erb
<!-- Pattern: {scope}-{component}-{element}-{type} -->
<%= button_to 'Delete User', user_path(@user),
    data: { testid: 'user-profile-delete-user-button' },
    method: :delete,
    class: 'btn btn-danger' %>

<%= link_to 'Edit Settings', edit_user_settings_path,
    data: { testid: 'dashboard-navigation-settings-link' } %>

<div data-testid="incident-status-resolved-indicator" class="status-badge">
  Resolved
</div>
```

### Collaboration with QA

**During feature planning (Recommended):**
- Work with QA during planning to understand testing requirements
- Ask QA which elements will need automation before you start coding
- Include test ID requirements in acceptance criteria
- Plan test attributes as part of the feature scope

**During feature development:**
- Include test IDs proactively for elements identified during planning
- Review QA feedback on PRs regarding test ID placement and naming
- Include test attribute additions in your PR descriptions

**Benefits of early collaboration:**
- Test infrastructure built with the feature
- No technical debt or retrofit work
- QA can start writing tests immediately when feature ships
- Better understanding between dev and QA teams

**When QA requests test IDs later (fallback):**
- Review and approve QA PRs that add test attributes to existing features
- Ensure test IDs don't conflict with existing attributes
- Test locally that new attributes don't break functionality

### Test-Friendly Development Practices

**Use semantic HTML when possible:**
```erb
<!-- ✅ Good: Semantic elements are easier to test -->
<button type="submit" data-testid="login-form-submit-button">
  Login
</button>

<!-- ❌ Avoid: Non-semantic elements need more specific selectors -->
<div class="btn" onclick="submitForm()" data-testid="login-form-submit-button">
  Login
</div>
```

**Provide stable identifiers:**
```erb
<!-- ✅ Good: Stable IDs for dynamic content -->
<% @incidents.each do |incident| %>
  <div data-testid="incident-<%= incident.id %>-card" class="incident-card">
    <h3 data-testid="incident-<%= incident.id %>-title">
      <%= incident.title %>
    </h3>
    <%= link_to 'View Details', incident_path(incident),
        data: { testid: "incident-#{incident.id}-details-link" } %>
  </div>
<% end %>
```

**Handle conditional rendering:**
```erb
<!-- ✅ Good: Consistent test IDs regardless of state -->
<% if user_can_edit?(@incident) %>
  <%= link_to 'Edit Incident', edit_incident_path(@incident),
      data: { testid: 'incident-edit-button' },
      class: 'btn btn-secondary' %>
<% else %>
  <span data-testid="incident-edit-disabled" class="disabled-text">
    Edit (Permission Required)
  </span>
<% end %>
```

## Best Practices

### Performance Considerations
- Test attributes have minimal impact on page size
- Use kebab-case for consistency: `data-testid="user-profile-edit-button"`
- Avoid overly long test IDs (keep under 50 characters)

### Code Reviews
- Include test ID additions in PR descriptions
- Ensure test IDs are descriptive and follow naming conventions
- Check that test attributes don't interfere with existing functionality

### Working with Legacy Code
- When modifying existing features, consider adding test IDs to elements you touch
- Don't feel obligated to add test IDs to unrelated code in the same file
- QA will handle systematic test ID addition through separate PRs

## Resources

- [E2E Testing Guidelines for QA](./e2e/README.md) - Comprehensive Playwright testing standards
- [Rails Form Helpers](https://guides.rubyonrails.org/form_helpers.html) - Official Rails documentation
