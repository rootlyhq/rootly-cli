# Frontend Architecture

## JavaScript

Write ES6/ES2015 compatible JavaScript. All DOM-binding JS uses Stimulus controllers, but complex state and shared logic are extracted into dedicated modules.

> **ActiveAdmin exception:** Stimulus is not available in ActiveAdmin pages. For ActiveAdmin, place JavaScript directly in ERB partials instead.

### Directory Structure

```
app/javascript/
├── controllers/       # Stimulus controllers
│   └── /              # Complex features get subdirectories (e.g., tiptap/)
├── models/            # Stateful JS classes (tooltip.js)
├── helpers/           # Shared utility functions (frequency.js, html_templating.js)
└── channels/          # ActionCable consumers
```

### Stimulus Controller Patterns

#### Delegated Controller (One Controller, Many Elements)

Use when many similar elements share behavior (tooltips, dropdowns, toggles):

```html
<div data-controller="tooltip">
  <button data-action="mouseenter->tooltip#show mouseleave->tooltip#hide"
          data-tooltip-content="Help text">?</button>
  <button data-action="mouseenter->tooltip#show mouseleave->tooltip#hide"
          data-tooltip-content="Another tip">?</button>
</div>
```

```javascript
import { Controller } from "@hotwired/stimulus"
import Tooltip from "models/tooltip"

const tooltips = new WeakMap()

export default class extends Controller {
  show(event) {
    const anchor = event.currentTarget
    if (!tooltips.has(anchor)) {
      tooltips.set(anchor, new Tooltip(anchor.dataset.tooltipContent))
    }
    tooltips.get(anchor).show(anchor)
  }

  hide(event) {
    tooltips.get(event.currentTarget)?.hide()
  }
}
```

Benefits: Fewer instances, works with dynamic content, less markup repetition.

#### Per-Element Controller (Controller per Element)

Use when elements have distinct behavior or heavy configuration:

```html
<div data-controller="chart" data-chart-type-value="line" data-chart-data-value="...">
</div>
```

```javascript
export default class extends Controller {
  static values = { type: String, data: Object }

  connect() {
    this.chart = new Chart(this.element, this.typeValue, this.dataValue)
  }

  disconnect() {
    this.chart.destroy()
  }
}
```

Use for: Charts, editors, complex forms, elements with unique configuration.

### Models for Stateful Elements

When an element has significant state (3+ variables, complex transitions), create a class in `models/`:

```javascript
// app/javascript/models/tooltip.js
export default class Tooltip {
  constructor(content, options = {}) {
    this.content = content
    this.element = null
    this.visible = false
    this.position = options.position || "top"
  }

  show(anchor) { /* create element, position, display */ }
  hide() { /* ... */ }
  destroy() { /* cleanup */ }
}
```

Controllers become thin wrappers that handle DOM events and delegate to models.

### Helpers for Shared Logic

Reusable pure functions live in helpers:

- frequency.js - debounce(), cancelable(), nextEventTick()
- html_templating.js - createFromTemplate()

```javascript
import { debounce, cancelable } from "helpers/frequency"
```

### Communication Patterns

- Outlets - Controller-to-controller (preferred when possible)
- CustomEvent - Cross-controller when outlets aren't feasible
- Stimulus values - Reactive data from data attributes

### Avoid Building HTML via String Concatenation

Stimulus controllers should not construct DOM elements by concatenating HTML strings. String-based HTML is fragile, prone to XSS if escaping is missed, and hard to review. Instead, use `<template>` elements defined in the server-rendered HTML, or render content server-side via Turbo Frames/Streams.

**Good Example — `<template>` element:**

```erb
<%# Server-rendered template that Stimulus clones %>
<template data-my-controller-target="cardTemplate">
  <div class="card" data-slot="name"></div>
</template>
```

```javascript
renderCard(data) {
  const template = this.cardTemplateTarget.content.cloneNode(true)
  template.querySelector('[data-slot="name"]').textContent = data.name
  this.listTarget.appendChild(template)
}
```

**Good Example — Turbo Frame (server-rendered):**

```erb
<%= turbo_frame_tag "group_list", src: groups_path, loading: :lazy %>
```

**Bad Example — string concatenation:**

```javascript
renderCard(group) {
  return `
    <div class="card" data-id="${group.id}">
      <p>${this.escapeHtml(group.name)}</p>
    </div>`
}
```

*Issue:* Manual escaping is easy to forget (especially in attributes), the markup can't be linted by ERB/HTML tools, and it duplicates structure that the server already knows how to render.

## CSS

We try to compose our CSS classes using Tailwind as much as possible. Should you need to add custom classes for your need,
you should be aware that all of our custom CSS classes get compiled from the `app/stylesheets/application.tailwind.css` file.
You have two ways to add custom classes:

1. **(Discouraged)** By adding them directly to the `application.tailwind.css` file.
2. **(Better)** By following our ViewComponent pattern: your custom styles should map to a view component, and a new file with the component name should be created and referenced in the `application.tailwind.css` file by using `@import`.
