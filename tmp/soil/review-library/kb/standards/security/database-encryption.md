# Database Encryption

Use application-layer encryption for secrets and personally identifiable information (PII) stored in database fields. Disk encryption protects physical media; it does not prevent plaintext exposure through database access, replicas, backups, or analytics tooling.

For Rails applications, use [Lockbox](https://github.com/ankane/lockbox). Use [Blind Index](https://github.com/ankane/blind_index) only when an encrypted value must support exact-match lookup or uniqueness.

## What Must Be Protected

Protect both of these categories:

1. **Secrets** are values that grant access, authority, or the ability to impersonate a system or user. Examples include API keys, access and refresh tokens, OAuth client secrets, webhook and signing secrets, private keys, session credentials, migration credentials, and credential-bearing URLs.
2. **PII** is any information about a person who can be identified directly from that value or reasonably identified by combining it with other available data. This includes names, personal or work email addresses, phone numbers, postal addresses, government identifiers, dates of birth, IP addresses, precise location, usernames, employee/member/customer identifiers, device identifiers, and authentication or sign-in telemetry. Free-form text, JSON, request bodies, and webhook payloads are PII when they contain these values.

Pseudonymous data remains PII when Rootly or another party can link it back to a person. Aggregated or irreversibly anonymized data is not PII when re-identification is not reasonably possible.

Encrypt secrets and PII when the application must recover the original value. If the application only needs to verify a password or token, store a one-way password hash or digest instead. An exception that leaves secrets or PII in plaintext requires a documented security review, including why encryption is not viable, who can access the data, retention limits, and compensating controls.

## Classify Before Implementing

Review the schema, models, and data flows. Column names alone are not enough, especially for `jsonb`, text, and serialized fields.

| Data | Required protection |
| --- | --- |
| Recoverable secrets: reusable credentials, API keys, OAuth tokens, webhook secrets, private keys | Encrypt. Prefer short retention and rotate after historical plaintext is removed. |
| PII: direct identifiers, linkable identifiers, contact details, device/network identifiers, and sign-in telemetry | Encrypt. Use a blind index only when exact lookup or uniqueness is required. |
| Free-form content, request bodies, webhook payloads, or integration JSON containing secrets or PII | Minimize and expire first; encrypt what must be retained. |
| Passwords and tokens used only for verification | Use a one-way password hash or digest, not reversible encryption. |
| Values requiring exact lookup or uniqueness | Encrypt and add a blind index over a consistently normalized value. |
| Values requiring range, prefix, fuzzy, or arbitrary JSON queries | Redesign the access pattern. Keep only non-sensitive derived metadata queryable; do not use deterministic encryption as a shortcut. |
| Public non-person identifiers, non-secret configuration, and one-way verifier digests | Do not encrypt without a specific threat model. |

Document the data owner, purpose, query requirements, retention period, and every secondary copy before migrating a field.

## Storage Rules

- Store ciphertext in a `text` column named `<attribute>_ciphertext`.
- Name blind-index columns `<attribute>_bidx`. Never use a blind index unless the application queries that attribute. Back it with a database index created concurrently, using a unique index when the attribute must be unique.
- Normalize only the lookup value used to calculate a blind index, and apply the same normalization to every lookup. Preserve the original encrypted value unless the product explicitly requires canonicalized storage. Never normalize case-sensitive secrets such as API keys or tokens.
- Do not place secrets in generic `data`, `metadata`, or settings JSON. Use an explicitly encrypted attribute instead.
- Treat generic text, JSON, and serialized fields as containing PII unless their allowed schema or contents demonstrate otherwise.
- Do not embed credentials in stored URLs. Separate and encrypt the credential-bearing component.
- Avoid encrypting an entire JSON document when the application queries inside it. Extract required queryable metadata into non-sensitive columns and encrypt the sensitive remainder.
- Keep encryption keys outside the database and source control. Use the service's approved secrets manager and support key rotation.
- Never copy plaintext into logs, error reports, job arguments, caches, search indexes, analytics, exports, or audit/version history. Explicitly exclude sensitive attributes from version tracking.
- Retain sensitive payloads and migration credentials only as long as the product or incident-response requirement needs them. Clear ephemeral credentials when a workflow finishes.

## Rails Pattern

```ruby
class IntegrationAccount < ApplicationRecord
  has_encrypted :api_token
end
```

```ruby
class AddEncryptedApiTokenToIntegrationAccounts < ActiveRecord::Migration[7.1]
  def change
    add_column :integration_accounts, :api_token_ciphertext, :text
  end
end
```

Add a blind index only for exact-match queries:

```ruby
class User < ApplicationRecord
  has_encrypted :email
  blind_index :email, expression: ->(value) { value.downcase }
end
```

The `expression` normalizes only the blind-index input; Lockbox preserves the original email value in ciphertext. Choose and document normalization for the product's lookup semantics. Do not copy this expression to case-sensitive secrets.

Add the blind-index column and its database index in schema migrations. Reconcile existing duplicates before creating a unique index.

```ruby
class AddEncryptedEmailToUsers < ActiveRecord::Migration[7.1]
  def change
    add_column :users, :email_ciphertext, :text
    add_column :users, :email_bidx, :text
  end
end

class AddUniqueIndexToUsersEmailBidx < ActiveRecord::Migration[7.1]
  disable_ddl_transaction!

  def change
    add_index :users, :email_bidx, unique: true, algorithm: :concurrently
  end
end
```

Use the application's established Lockbox wrapper and key configuration where one exists. Do not introduce per-model encryption key conventions.

## Migrating a Plaintext Field

Encryption migrations must tolerate old and new application versions running at the same time. Follow the general [safe database migration standard](../rails/migrations.md) and ship schema changes separately from application changes.

1. Inventory reads, writes, queries, uniqueness constraints, exports, audit history, logs, jobs, caches, and retained copies.
2. Add nullable ciphertext and, only if required, blind-index columns in a schema-only PR. Create the blind-index database index concurrently; use `unique: true` when uniqueness is required.
3. Deploy `has_encrypted :attribute, migrating: true` (and `blind_index :attribute, migrating: true` when needed). This dual-writes new values and falls back to legacy plaintext while rows are migrated.
4. Backfill through `Lockbox.migrate(Model)` or the application's equivalent bounded, resumable job. Do not use raw SQL to copy plaintext into the ciphertext column or print values in progress logs.
5. Verify row counts, decryptability, normalization, lookup behavior, and error rates without exposing plaintext.
6. Switch reads to ciphertext only. Stop plaintext writes and clear the legacy value.
7. Remove the plaintext column in a later migration after the rollback window closes.
8. Remove plaintext from secondary stores and historical versions. Rotate reusable secrets that may have existed in backups or audit history.

Use an idempotent backfill that skips rows already encrypted and can safely resume after failure. Large backfills belong in the application's data-migration or background-job framework, not a long-running schema migration.

## Pull Request Checklist

- [ ] The field is classified as a secret, PII, both, or neither; its owner, query needs, retention, and secondary copies are documented.
- [ ] Plaintext is absent from storage, logs, jobs, caches, analytics, exports, and version history.
- [ ] Tests prove database values do not contain plaintext and encrypted values still round-trip.
- [ ] Blind-index lookup and uniqueness tests cover normalization, if a blind index is used.
- [ ] The rollout supports mixed application versions and a resumable backfill.
- [ ] Verification, rollback, historical cleanup, and credential rotation are defined.

## References

- [Lockbox documentation](https://github.com/ankane/lockbox)
- [Blind Index documentation](https://github.com/ankane/blind_index)
- [OWASP Cryptographic Storage Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Cryptographic_Storage_Cheat_Sheet.html)
