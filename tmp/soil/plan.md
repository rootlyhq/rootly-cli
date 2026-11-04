# Plan: IR-6588 (CLI: workflows list / workflows run)

**Already done, out of scope:** `rootly workflows list` and `rootly workflows run <id-or-slug> --incident=<id>` shipped in rootly-cli#56 (commit 14c8081, first released in v0.5.0). This lane plans nothing for them.

## Work items

### 1. Keep the diff against master empty (ac-1)
- **Change:** none. No source, test, or Makefile edits. The lane's only artifacts live under the untracked `tmp/soil/`.
- **Delivered on master by commit 14c8081 (#56), not by this lane:** `internal/cmd/workflows/{workflows,list,run}.go` (+ `cmd_test.go`), the workflow methods in `internal/api/client.go` (+ `internal/api/client_workflows_test.go`). A commit here would break the criterion that the diff stays empty.
- **Proof:**
  - `git diff master --stat` prints nothing (mechanical, re-run at verify).
  - Supporting evidence that the shipped code works (not new tests): `go test ./internal/cmd/workflows/... ./internal/api/...` passes on HEAD, and `make build && bin/rootly workflows --help` lists `list` and `run`, with `run` requiring `--incident`.

### 2. Close-as-done decision: a person makes it (ac-2, kind: human)
- **Change:** none in code. No diff can deliver this. The decision to close IR-6588 or comment on it belongs to the human, because closing posts as them.
- **Action for the person:** decide whether to close IR-6588 with a comment linking rootly-cli#56 and pointing users to v0.5.0+ (see `human_action` in intent.json).
- **Lane's part:** bring the proposal and evidence as an ask, and never close the ticket or comment on Linear or GitHub itself. The owner was already asked and chose "Leave the ticket open, no comment". That answer stands, and nothing gets posted.
- **Proof:** the recorded ask and answer in the lane's records. No Linear or GitHub write appears in the lane's tool calls.

## Order
1 → 2. Both are independent and neither produces a commit.
