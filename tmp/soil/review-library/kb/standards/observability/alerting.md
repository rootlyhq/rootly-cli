# Alerting Guidelines

As part of improving our operational excellence, this document defines a baseline standard for alerting at Rootly.

Alerts provide asynchronous notifications for system conditions. They ensure that humans don't have to stare at graphs and dashboards all day. The overall objective is to ensure we are alerted to important issues while minimizing alert fatigue.

## TL;DR

Your alerts should:
- include either a link to a playbook, or clear next steps, in the message
- be actionable
- correlate to active or impending customer pain
- have Datadog tags attached (at least service and env for production/staging alerts)

## What to Monitor?

At a minimum, when you're building new infrastructure, products, and/or features, you should be aiming to answer the following questions with the metrics you collect:

- **What is the user-facing latency of the system?**
  - e.g. how long does it take to create an incident channel when a customer needs one?
- **What is the demand on the system?**
  - e.g. how many requests per second is the system handling, is it anomalous?
- **What errors are being encountered in the system?**
  - e.g. how many 500s are we serving, how often is our internal API throwing an exception?
- **How close to capacity is the system?**
  - e.g. memory usage, CPU load, I/O waits

If you can only answer and monitor just these four cases, you should still have a good foundation for knowing when something goes awry.

## Service Level Objectives

Service Level Objectives (SLOs) are loosely related to alerting. At Rootly, they should define a reliability objective for a specific, high level user journey (e.g. "99.9% of incident channels are created successfully"). Because they are tied to critical user journeys, we should have a small set of them per product and expect them to be topline health metrics, rather than tracking the health of internal infrastructure.

We don't alert on the SLO percentage specifically, but should alert on the Burn Rate.

### Error Budget

An SLO of 99.9% gives ~8h 41m of "downtime" per year. This is the "error budget", which offers a mechanism for taking risk while accounting for the overall reliability objective. Inspecting the error budget on a monthly or quarterly basis allows teams to decide how much risk to accept in rollouts, maintenances, and other changes to the system. For example, if there isn't much error budget left in a quarter, a risky maintenance might be deferred to avoid breaching the SLO.

### Burn Rate & Alerting

An SLO's burn rate indicates how fast its error budget is being consumed. SLO alerts are almost always burn rate alerts - and two types are usually needed.

The first is a more typical, short-duration alert: did we burn a lot of the budget in the last 1-5 minutes? These often indicate total outages or other major incidents.

The second inspects the burn rate over a much longer duration. A low but continuous rate of errors could still breach the SLO at the end of a quarter: alerting on a slow burn that's forecasted to cause a breach allows the team to respond early and stop the bleeding.

If you are considering anything beyond these two alerts for an SLO, let's chat about it. It's rare to need more.

### Service Level Agreements

These are out of scope for our alerting/monitoring philosophy: in practice, they are built in collaboration with legal and sales to define a specific agreement, including remedies and a clear definition of what constitutes a miss.

## Key Alert Principles

### Is the alert ignorable?

If yes, are there modifications you can make to thresholds or make the condition more specific? An alert that can be ignored will be, even when it's important, because it becomes background noise.

### Does the alert imply definite impact (active or impending)?

By way of example, "Disk usage exceeds 80%" does not imply a customer-facing impact. It's potentially problematic, but probably not that useful to receive as a responder. In this example case, a forecast alert that ties back to a "File uploads" feature would be potentially more actionable by a responder (with careful consideration for the next point about definite action to take).

### Is there a definite action to be taken for the alert?

The answer to this should always be "yes", otherwise the alert becomes ignorable.

This should be provided as a playbook or similar guidance that is attached to the alert itself (via the alert message, usually). Don't ask responders to go hunting documentation as their first action.

### Will this alert fire in tandem with others?

If "yes", you might be either:
- duplicating an existing alert, or
- alerting at multiple levels of infrastructure (e.g. a Postgres alert for locks vs an application latency alert)

Multiple alerts for the same root cause can be overwhelming and contribute to alert fatigue. In our current environment where oncallers generally operate across the full stack, consider these cases carefully. (in an environment with an "infra" team, it may be more acceptable to alert the infra + app teams)

## Should I Page?

I like to use "The 3AM Test" for this decision: waking an engineer is expensive (physically, emotionally, mentally, and so on), so **make it count**. We want to be waking people up for critical events that are immediately actionable. These guidelines help pass the test:

- Does the alert clearly correlate to an active customer-visible issue, or an impending disaster?
- Does the alert have a clearly defined action for mitigation?
- Is there a playbook for the alert that a half-awake engineer can run?

**If you get paged and can't take any action, seriously consider whether the page is appropriate.**

## Should I Slack?

Yes, but only if a response time exceeding 8 hours is acceptable. Slack cannot be actively monitored by an oncaller for 24 hours, so in a single-oncaller environment, there will always be a window in which an alert is "ignored" for an extended duration.

If this is not acceptable, see [Should I Page?](#should-i-page) to determine whether paging is justified for the alert.

The vast majority of alerts do meet this specification.

## Should I \<Email/Smoke Signal/\*\>?

Ideally emailed alerts are only for early testing. It is better for everyone to have two clear alert destinations (page, Slack), than to have a myriad of destinations. It's easier for the team to check, it's easier for spectators to follow along, and it's one less choice you have to make when defining your alerts.

## Alerts vs "Signal" Monitors

In some cases a monitor might not meet the standard or criticality threshold to actually trigger an alert. These frequently indicate some sort of condition that may be interesting in the context of a larger problem, but are not by themselves immediate problems.

In Datadog, these can be configured without notification destinations, allowing them to be enumerated and presented in dashboards and notebooks without adding noise to alerting destinations.

For example, Datadog's "Watchdog" alerts almost always fit more as "Signals"; they are difficult to tune and don't meet the standards set out above. However, they are frequently interesting when debugging a larger issue, so presenting a list of them on a service health dashboard can help with troubleshooting.
