# Task executor with dependencies

Deterministic in-process executor for a plan of simulated tasks. A task runs only after every dependency has succeeded. A failure blocks dependents and does not stop independent work. An invalid plan is rejected before any action runs.

**Focus:** backend domain rules (library + command-line demo).

**Time:** one working session for the library, tests, and CLI. Replace this with your own figure before you submit.

## Requirements

Go 1.22 or newer.

```bash
go test ./...
go run ./cmd/taskexecutor examples/spec-plan.txt
go run ./cmd/taskexecutor examples/filing-plan.txt
```

No module download is required. The program does not open a network connection, a database, or a shell.

## Plan file

Blank lines and lines starting with `#` are ignored.

```text
<id> <ok|fail|panic> [reason words...] [<- dep,dep]
```

`ok` cannot carry a reason. `fail` and `panic` can. Dependencies are comma-separated and come after `<-`.

Identifiers are ordered as raw Go strings (byte order), not as numbers. `task-10` runs before `task-2`.

## Demo — document example

`examples/spec-plan.txt` is the example from the case. Expected order: A, B, C, E. D is blocked by B and its action does not run.

```text
executed:
- A
- B
- C
- E
tasks:
- id: A
  status: success
- id: B
  status: failed
  reason: disk full
- id: C
  status: success
- id: D
  status: blocked
  blocked_by: B
  reason: blocked by B (failed)
- id: E
  status: success
```

## Demo — a different scenario

`examples/filing-plan.txt` is the scenario to walk through in the video. It is fictitious filing work, not a real company workflow.

Hand-computed expectation before running:

1. `collect-records` and `notify-stakeholders` have no dependencies. `collect-records` is smaller, so it runs first and succeeds.
2. `notify-stakeholders`, `prepare-annual-report`, and `validate-standing` are ready. String order runs `notify-stakeholders`, then `prepare-annual-report`, then `validate-standing`.
3. `validate-standing` fails with `standing is delinquent`.
4. `submit-annual-report` depends on that failure, so it is blocked and does not run. The notification already ran because it does not depend on the standing check.

```bash
go run ./cmd/taskexecutor examples/filing-plan.txt
```

```text
executed:
- collect-records
- notify-stakeholders
- prepare-annual-report
- validate-standing
tasks:
- id: collect-records
  status: success
- id: notify-stakeholders
  status: success
- id: prepare-annual-report
  status: success
- id: submit-annual-report
  status: blocked
  blocked_by: validate-standing
  reason: blocked by validate-standing (failed)
- id: validate-standing
  status: failed
  reason: standing is delinquent
```

A cycle prints the reason on stderr and exits 1. Nothing runs, including tasks outside the cycle:

```bash
printf 'X ok <- Y\nY ok <- X\nZ ok\n' > /tmp/cycle-plan.txt
go run ./cmd/taskexecutor /tmp/cycle-plan.txt
```

## Tests worth reading

`go test ./...` covers the rules. Three of them carry the design:

1. **`TestDocumentExample`** — failure, blocking, and independent work together. B fails, D stays blocked even though D's action would succeed, and C and E still run, in the order A, B, C, E. This catches an executor that aborts on the first error, or that runs a blocked action.
2. **`TestStringOrderNotNumericOrder`** — the tie-break boundary. Ready tasks `m`, `task-10`, and `task-2` run in that order. This catches numeric sorting (`task-2` before `task-10`) and "keep the input order".
3. **`TestCycleRejectsTheWholePlan`** — X depends on Y and Y depends on X, while Z does not. The plan is rejected and Z's action is not called. This catches validation that reports a cycle but still runs the independent part. `TestMissingDependencyRunsNothing` is the same guarantee for a missing dependency.

## Decisions

**Which direct dependency to name when several are blocked.** The case only requires at least one direct dependency that failed or was blocked. Leaving the choice open would make two runs of the same plan disagree. The report names the smallest such identifier. `TestFailureBlocksDescendantsAndIndependentWorkContinues` locks that: F depends on B and C, both end badly, and F records `blocked by B (blocked)` because B is the direct dependency and B is smaller than C. The reason names the direct dependency, not the earlier root failure.

**A panic fails that task and the process continues.** The case says an error or a thrown exception marks the task failed and must not stop the executor. In Go the second case is a panic, so `runAction` recovers. The alternative was to let the panic crash the process, which is simpler and wrong for this case. `TestPanicIsATaskFailure` checks the recorded reason and that the next independent task still runs.

An assistant drafted the first implementation from the case text. One suggestion that fits "keep going" is to run every ready task in its own goroutine. I did not take it. The case says to run one task at a time, forbids parallelism, and requires a total order when several tasks are ready. Concurrent actions would make that order untestable. I checked the draft by writing the expectations from the rules first, then running `go test ./...` and both example files above.

## Limits

Out of scope, on purpose: retries, schedules, disk, distribution, and parallelism. A durable workflow runner would persist each step and could run independent tasks together. This library does neither. It also keeps no memory between `Execute` calls.

If two task identifiers are duplicated, the plan is rejected as invalid and cycle detection is skipped, because the graph is ambiguous. Self-dependencies are reported on their own and are not also reported as a one-node cycle.

## Not done

The explanation video is not recorded. Before sending, publish this directory as its own repository (the case catalog asks you not to open a pull request there), put the commit hash and the video link in this README, and replace the time note at the top with the time you actually spent.
