# Redis

## App-Level Default TTL

The Rails cache store is configured with a **1-day default TTL** in both staging and production (`expires_in: 1.day` in `config/environments/`). Any `Rails.cache.write` or `Rails.cache.fetch` call without an explicit `expires_in` will inherit this default.

This is a safety net — **you should still set explicit TTLs** on all cache entries to match their intended lifetime. The 1-day default prevents accidental unbounded growth if an `expires_in` is forgotten.

> **Note:** The default TTL applies to `Rails.cache` only (the cache Redis). Sidekiq Redis keys (locks, queues, scheduled jobs) are managed separately and are not affected.

## Every Key Should Have an Explicit TTL

**All `Rails.cache.write` calls should include an explicit `expires_in`.** While the app-level 1-day default provides a safety net, relying on it silently masks intent — a reader can't tell whether the author chose a 1-day TTL or simply forgot.

```ruby
# Good — explicit TTL, intent is clear
Rails.cache.write(cache_key, data, expires_in: 24.hours)

# Acceptable — inherits 1-day default, but intent is unclear
Rails.cache.write(cache_key, data)
```

**Rationale:**
- Redis is an in-memory store with a fixed `maxmemory` budget
- Keys without explicit TTLs rely on the app default, which may not match the data's actual lifetime
- When Redis hits `maxmemory`, eviction policies kick in and cause unpredictable cache misses or job failures
- An explicit TTL makes the cache contract self-documenting

## TTL Guidelines by Use Case

| Use Case | Recommended TTL | Example |
|----------|----------------|---------|
| Request/session cache (filters, UI state) | 1-24 hours | User filter preferences |
| Background job locks (`sidekiq-unique-jobs`) | 1 hour (global default) | `lock_ttl: 1.hour` |
| Temporary progress tracking | 2-24 hours | Bulk operation progress |
| Scheduled email/inbound mail cleanup | 7 days | `ActionMailbox.incinerate_after` |
| External API response cache | 1-24 hours | Salesforce account data |
| Reindex tokens / short-lived coordination | 1-5 seconds | Search reindex dedup |

## Rails Cache (`Rails.cache.write`)

Always pass `expires_in`:

```ruby
# Good — explicit TTL
Rails.cache.write("user_filters:#{membership_id}", filters, expires_in: 24.hours)
Rails.cache.write("api_response:#{account_id}", data, expires_in: 1.hour)

# Good — fetch with TTL
Rails.cache.fetch("expensive_query:#{team_id}", expires_in: 15.minutes) do
  Team.find(team_id).compute_expensive_thing
end

# Bad — no expiry, lives forever
Rails.cache.write("user_filters:#{membership_id}", filters)
```

## Sidekiq Unique Jobs (`lock_ttl`)

Every job using `sidekiq-unique-jobs` locks must have a `lock_ttl`. A global default of 1 hour is configured, but jobs should set explicit values matching their expected duration:

```ruby
# Good — explicit lock_ttl matching job duration
sidekiq_options lock: :until_executed, lock_ttl: 10.minutes

# Good — using the UniqueJob concern with TTL
discard_duplicate_jobs lock_ttl: 5.minutes

# Bad — always set an explicit lock_ttl regardless of job duration
sidekiq_options lock: :until_executed
```

**Never disable the global `lock_ttl` default.** Without it, orphaned lock digests accumulate in the `uniquejobs:digests` sorted set and consume memory indefinitely.

## ActionMailbox Incineration

ActionMailbox schedules `IncinerationJob` for every inbound email. The default is 30 days, which can create 70K+ scheduled jobs in Redis:

```ruby
# Good — 7 days is sufficient for debugging inbound email issues
config.action_mailbox.incinerate_after = 7.days

# Bad — default 30 days creates excessive scheduled job volume
# (no config = 30 day default)
```

## Debugging Redis Memory

When investigating Redis memory issues, use these diagnostic queries:

```ruby
# 1. Check memory usage
Sidekiq.redis do |conn|
  info = conn.call('INFO', 'memory')
  puts info.lines.grep(/used_memory_human|maxmemory_human|mem_fragmentation/)
end

# 2. Find large keys (O(1) per key, safe to run)
Sidekiq.redis do |conn|
  cursor = 0
  loop do
    cursor, batch = conn.call('SCAN', cursor, 'COUNT', 5000)
    batch.each do |k|
      type = conn.call('TYPE', k)
      size = case type
             when 'zset' then conn.call('ZCARD', k)
             when 'list' then conn.call('LLEN', k)
             when 'set' then conn.call('SCARD', k)
             when 'hash' then conn.call('HLEN', k)
             else 0
             end
      puts "#{type} #{k}: #{size}" if size > 10_000
    end
    break if cursor.to_i == 0
  end
end

# 3. Identify job classes in the scheduled set
classes = Hash.new(0)
Sidekiq::ScheduledSet.new.each { |job| classes[job.klass] += 1 }
classes.sort_by { |_, v| -v }.first(10).each { |k, v| puts "#{k}: #{v}" }
```

**Avoid `MEMORY USAGE` per-key scans** — they inspect internal allocations and will hang on large datasets. Use `ZCARD`/`LLEN`/`SCARD`/`HLEN` instead (O(1) each).
