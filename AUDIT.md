# AUDIT.md — ZJDDNS audit playbook

Reusable playbook for full ZJDDNS audits. Execute the next audit round accordingly: **gate baseline → parallel audit → Sprint fixes → Round C re-check → doc backfill**.

Goal: **audit round by round until a round produces no new findings**. Every fix must re-run regression on its impact surface + lint gates; at the end of each round, land new lessons as present-tense constraints into AGENTS.md and the corresponding sections of this file (no round battle reports, no report archives — commit history is the record); never audit the same mistake twice.

## 1. Environment preparation (gate baseline)

```sh
go build ./...                    # zero compile errors
go test ./... -count=1            # all pass (-count=1 prevents cached false-greens)
go test -race ./...               # race all green (project runs sequentially, should have zero races)
go fix ./... && golangci-lint fmt && golangci-lint run   # zero warnings (same order as the pre-commit hook)
```

- The baseline must be fully green before the audit starts — audit findings diff against the baseline, not against noise.
- The pre-commit hook (`scripts/pre-commit`) automatically runs fix → fmt → partial-staging safe re-add → run on every commit; the hook's partial-staging contract: **only re-add files the formatter actually changed** (worktree blob hash before/after comparison).
- This project has no benchmark baseline (low-frequency ticker scenario; microbenchmark noise outweighs the benefit). Performance-dimension audit conclusions use **structural criteria**: API calls per tick (content-compare avoids no-ops), detection count per type (union-type merging), no repeated construction on hot paths. If high-frequency scheduling is introduced in the future, establish a baseline before talking about regression.

## 2. Audit dimensions (scan checklist)

Dispatch Explore read-only agents in parallel + self-review, covering:

0. **Correctness/robustness**: panic surface (every offset advance in the hand-written DNS parsing must be bounded), swallowed errors (verify the reason for each item in the `_ =` list), discrimination between lookup failure and "record missing" (first pattern in §5), escaping and family validation of config values before they enter network calls.
1. **Consistency**: comments English-only (grep non-ASCII), godoc syntax, decorder order (`type → const → var → func`), Go 1.27 idioms (`new(expr)`, `min/max`, `range-over-int`, `slices`).
2. **Doc drift**: cross-validate all .md × code — symbol existence, behavioral assertions, validity of referenced filenames; phantom API references and casually-written false assertions like "concurrent" both fall into this category. "Doc/implementation inconsistency is treated as a bug."
3. **Logging discipline**: check the prefix at every call site (DDNS/CLOUDFLARE/IPDETECT/CONFIG), level semantics (Error=component failure, Warn=fallback, Info=state changes only, Debug=routine), state-change Info lines carry record name + IP provenance.
4. **Constants**: numeric literal scan (`\d{3,}` and bare numbers at the wire layer), single ownership per constant, Cloudflare TTL tiers consistent with docs.
5. **Test quality**: assertions must not be tautological, error paths have cases, coverage gaps explicitly registered (`go test -cover ./...` snapshot); pure functions must be testable and tested.
6. **Engineering support**: CI workflows (existence of test gates, whether failures are swallowed, action versions), dependabot ecosystem coverage, scripts POSIX/PS twin consistency, Dockerfile, config.example.json sync gate.

## 3. Execution

Scaled to 3 package-level + 7 cross-dimension passes, executable by parallel agents (read-only); the lead auditor synthesizes and ranks:

```
Phase 1: package-level (3 parallel)
├── Foundation: internal/log + internal/ipdetect (hand-written DNS byte layer, line by line)
├── Domain:     config + ddns (validation chain, Runner orchestration, static IP)
└── Edge:       providers/* + cmd/zjddns (API clients, wiring, loop)

Phase 2: cross-dimension (7 parallel, grep-driven) → 0-6 of §2
Phase 3: Synthesis → ranking → theme analysis → fix plan (goes straight into commits, no report archive)
```

Each finding records: file + line number, severity (CRITICAL/HIGH/MEDIUM/LOW), category, problem, risk, fix direction. False-positive cost is controlled by the "verify the code item by item before reporting" discipline.

## 4. Sprint fix strategy

| Sprint | Scope | Criteria |
|--------|------|------|
| 1 | CRITICAL | Fix immediately — data corruption, crashes, panic, security bypass |
| 2 | HIGH | Silent failures, races, resource exhaustion, missing gates |
| 3 | MEDIUM + LOW | Consistency — validation, docs, comments, micro-optimizations |

- Commit discipline per AGENTS.md §Git Discipline: **one commit per class of fix**, subject line describes the fix itself (not the audit phase), may carry a numbered reference (`(H1)`).
- Fix priority: one-line fixes → pattern fixes → logic rewrites → dead-code removal.
- **Round C re-check**: after fixes, read diffs commit by commit to catch regressions introduced by the fixes themselves, re-run full gates; wrap up only after two consecutive rounds with no new findings.
- `go run ./cmd/zjddns -generate-config | diff - config.example.json` is a quick behavioral-compatibility regression.

## 5. Root-cause patterns (ZJDDNS corpus)

Recurrence makes a pattern; append new findings in this format:

| Pattern | Root cause | Prevention |
|------|------|----------|
| **Swallowed error = state misjudgment** | `recordID, _ := c.RecordID(...)` conflates API failure with "record does not exist" — upsert becomes duplicate creation, delete becomes silent success | Lookup failure must propagate; "empty value = does not exist" only holds when there is no error |
| **Unescaped URL concatenation** | `fmt.Sprintf` inserts user config (zone_name/record_name) directly into the query string | Unify on `url.Values.Encode()`; tests include reserved-character cases |
| **CI without quality gates** | Workflow is build+push only, tests/lint rely entirely on local self-discipline; `continue-on-error` swallows platform failures into green | Release must have a test/lint job dependency chain; ban continue-on-error on the release chain |
| **Doc drift** | After behavior removal, docs retain stale descriptions, referenced filenames go stale, package comments contradict the implementation | Grep docs on every behavior change; during audits verify behavioral assertions one by one |
| **Comment language contamination** | Chinese comments in test files violate English-only | Audit greps CJK ranges |
| **`_` discard without comment** | Contract lives only in interface godoc, call sites have bare `_ =` | Inline reason at every site (except idiomatic defer-Close) |
| **Silent script failure** | `grep '\s'` not recognized on macOS → empty parse → sed no match → pretends success; partially staged files get fully re-added by the hook | Validate parse results against semver shape first, grep-verify after substitution, hook uses blob hash to judge "the formatter actually changed it" |
| **Exit code semantics** | Fatal config error does `log.Errorf` then `return` → exit 0, Docker restart policy goes blind | Startup failures uniformly go through `fatalStartupError` (exit 1) |
| **Comments ahead of facts** | Docs promise "prints 🌐" and "runs concurrently" — never existed in the code or already removed | Every behavioral assertion must be greppable; write docs to the current state, not to intent |

## 6. Known trade-offs and by-design behavior (do not report as bugs)

- **Dockerfile build-time wget of the CA bundle** (curl.se): deliberate owner design (registered in AGENTS.md §Behavior preservation); do not "simplify" into copying inside the build image, do not add checksums.
- **No context.Context**: all timeouts go through `SetDeadline` / `http.Client.Timeout`; this is a registered design trade-off — new blocking I/O must be bounded the same way.
- **Zero goroutines**: the sequential scheduling loop is the baseline; `internal/log`'s lock-free writer depends on this. Before introducing concurrency, add writer locks and race tests first.
- **API list endpoints not paginated** (zone/record fetch the first page): the DDNS own-token scenario has controllable counts; explicit errors beat silent truncation — when exceeded, the upper-layer API error is perceivable.
- **parseDNSResponse does not validate QDCOUNT**: the ID/QR/rcode triple gate comes first, TXT content additionally passes family validation; misaligned parsing inevitably errors — not a viable path.
- **Non-root container not enabled**: scratch has no /etc/passwd, and the mounted root:600 config.json would become unreadable — staying root is a deployment-compatibility decision.
- **Default level string defined in two places** (`log.DefaultLevel` and `config.DefaultLogLevel`): both packages are zero-dependency foundations and don't import each other; `cmd` already has a default via `ParseLevelFilter`, config's constant serves the example output.
- **`Runner.Run`'s invalid-mode Error branch is unreachable**: config.Validate already guards it; kept as a programming-error backstop.
- **`staticIP` wedged between `*Runner` methods**: idiomatic Go, decorder doesn't constrain it; receiver grouping is a visual convention, not a hard rule.

## 7. Registered test gaps (starting items for the next audit round)

- **`Runner.detector` not injectable**: the auto-detection path and detection-failure path can't be tested offline — after interfacing the detector, add the four case classes auto/DNS/HTTP/failure.
- **Local fake DNS server tests**: loopback fake servers via `net.Listen("udp"/"tcp")` cover TC→TCP retry (the transport layer in dnsquery.go, currently the main coverage gap); loopback doesn't violate the "no external network" test discipline.
- **Versioned releases**: CI currently pushes only `latest`; tag-triggered versioned images + GitHub Release remain a gap (new item, needs the owner to decide the release process).
- **parseDNSResponse truncation-shape cases**: the corruption branches for question/answer/TXT (:130/:140/:147/:155) have no hand-built cases.
- **`internal/log` supplementary tests**: `sanitizeLogMessage` (including NUL/DEL/multibyte), `Level.String`, SetLevel clamping, Warn always-pass rule.

## 8. Closing loop

1. Fix → full gates (§1) → impact-surface regression (affected packages `-count=1`).
2. Round C re-check (§4): wrap up after two consecutive rounds with no new findings.
3. Each round, land new lessons into docs as present-tense constraints: AGENTS.md (conventions + one-line lessons), this file (§5 root-cause patterns / §6 trade-off list / §7 gap additions).