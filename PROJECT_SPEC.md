# Cellar Run Control Specification

## Goal

Cellar Run Control is a local Go HTTP service for a small production cellar.
It coordinates approved formulas, vessel reservation, active fermentation
runs, gravity observations, conditioning, completion, and vessel release.

The project is a healthy initialization baseline. It contains no intentional
defects, no model traces, and no automated tests. Verification is intentionally
deferred to a later engineering task.

## Users

- Production planner: creates formulas, updates process steps, and approves a
  formula before it can be used.
- Cellar operator: registers vessels, creates runs, reserves a vessel, and
  starts fermentation.
- Fermentation observer: appends timestamped gravity observations and moves a
  run through conditioning and completion.
- Integration client: calls the stable JSON API from a local production script.

## Entities

### ProcessConfiguration

A process configuration is a versioned, immutable-after-publication cellar
standard. Each version belongs to a stable lowercase key (for example,
`winter-stout`) and contains fermentation temperature bounds, minimum and
maximum planned fermentation days, and the observation counts required for
conditioning and completion.

States are `draft`, `published`, and `withdrawn`.

- A key has at most one draft at a time.
- Creating the first draft assigns `config_version` 1; later drafts for the
  same key receive the next integer.
- Temperature bounds are 5 to 40 Celsius, and the minimum cannot exceed the
  maximum.
- The minimum fermentation duration is 0 to 120 days; the maximum is 1 to 120
  days and must not be below the minimum.
- Observation thresholds are 1 to 500; completion cannot require fewer
  observations than conditioning.
- Publishing captures the effective timestamp and publication time. Published
  values cannot be modified. A future effective timestamp is valid but does not
  govern a run until that time.
- Withdrawal marks a published version unavailable for future run starts. It
  never mutates or invalidates a snapshot already bound to a run.
- When multiple published versions for a key are effective at the same time,
  selection uses the highest `effective_at`, then `config_version`, then
  publication time, then ID as deterministic tie-breakers.
- The run decision lists every version in the family, its state, timestamps,
  eligibility, and the reason it was or was not selected.

### Formula

A formula contains an identifier, normalized name, style label, target original
gravity, target final gravity, maximum fermentation temperature, minimum and
maximum planned days, ordered process steps, lifecycle state, version, and
timestamps.

States are `draft`, `approved`, and `retired`.

- Names are unique after case folding and whitespace normalization.
- Target original gravity is in the inclusive range 1.030 to 1.120.
- Target final gravity is positive, lower than target original gravity, and
  differs from it by at least 0.005.
- Maximum fermentation temperature is in the inclusive range 5 to 40 Celsius.
- Planned duration is 1 to 120 days and the minimum cannot exceed the maximum.
- A formula has 1 to 24 process steps.
- Step sequence values must be contiguous and start at 1.
- Step temperatures are in the inclusive range 5 to 90 Celsius.
- Step hold times are 1 to 1440 minutes.
- `draft -> approved` requires a complete valid step list and a selected
  process configuration key that has at least one published or withdrawn
  version.
- A draft chooses the process configuration key before approval. Approved
  formulas retain that key but do not pin the version; each run resolves and
  snapshots the effective version when fermentation starts.
- An approved formula is immutable.
- `approved -> retired` is allowed only when no run currently uses it.

### Vessel

A vessel contains an identifier, unique display code, capacity in liters,
lifecycle state, active run identifier, version, and timestamps.

States are `available`, `reserved`, `in_use`, `cleaning`, and `retired`.

- Capacity is in the inclusive range 10 to 5000 liters.
- Only an available vessel can be reserved.
- A reserved or in-use vessel must identify exactly one active run.
- Completion moves an in-use vessel to cleaning.
- Cleaning can be finished only when the associated run is complete or
  aborted.
- Finishing cleaning releases the vessel to available.
- A vessel with an active run cannot be retired or directly reserved.

### FermentationRun

A run contains an identifier, unique run code, an approved formula identifier,
an optional vessel identifier, planned volume, notes, lifecycle state,
timetable, ordered observations, final metrics, version, and timestamps.

States are `planned`, `reserved`, `fermenting`, `conditioning`, `completed`,
and `aborted`.

- Planned volume is greater than zero and cannot exceed vessel capacity.
- Only an approved formula may be attached to a run.
- Run creation records the formula's process configuration key.
- `reserved -> fermenting` resolves the currently effective published
  configuration for that key, stores a complete copy of the selected values,
  and records a decision containing the evaluated time, candidate count, and
  deterministic selection basis.
- Observation temperature, conditioning observation count, minimum elapsed
  fermentation time, completion observation count, and maximum elapsed
  fermentation time are judged only against the configuration snapshotted on
  the run. Later publication or withdrawal cannot change those checks.
- A run cannot start when no published version for its key is effective at the
  start time.
- `planned -> reserved` atomically reserves an available vessel.
- `reserved -> fermenting` requires that the vessel still belongs to the run.
- `fermenting -> conditioning` requires at least two observations.
- `conditioning -> completed` requires at least three observations.
- Completion computes apparent attenuation and estimated alcohol by volume from
  the formula target original gravity and the last observation.
- Completion moves the vessel to cleaning without losing its run association.
- A non-completed run can be aborted. Aborting before fermentation releases an
  available vessel; aborting after fermentation schedules the vessel for
  cleaning.
- Completed and aborted runs are immutable.
- A run can carry at most 500 observations.

### GravityObservation

An observation contains a run identifier, contiguous sequence number, observed
time, gravity, temperature, optional note, and receipt time.

- Gravity is in the inclusive range 0.980 to 1.200.
- Temperature is in the inclusive range -5 to 60 Celsius.
- Temperature is first checked against the physical instrument range -5 to 60
  Celsius and then the bound process configuration range.
- Observed time begins at or after the run start.
- Observation times strictly increase within a run.
- Gravity must not rise more than 0.010 above the immediately preceding value.
- Observation sequences are contiguous and start at 1.

## Connected Workflows

### 1. Formula Approval

Public entry points are:

- `POST /v1/formulas`
- `PUT /v1/formulas/{id}/steps`
- `PUT /v1/formulas/{id}/process-config`
- `POST /v1/formulas/{id}/approve`
- `GET /v1/formulas/{id}`

The client creates a draft, sets its process step list atomically, approves it,
and reads the approved record. Invalid ranges, duplicate names, empty steps,
and repeated approval return stable error codes without partial writes.

This path crosses HTTP protocol handling, application orchestration, domain
rules, and snapshot storage.

### 2. Versioned Process Configuration

Public entry points are:

- `POST /v1/process-configs`
- `GET /v1/process-configs`
- `GET /v1/process-configs/{id}`
- `PUT /v1/process-configs/{id}`
- `POST /v1/process-configs/{id}/publish`
- `POST /v1/process-configs/{id}/withdraw`

A supervisor creates a draft for a stable configuration key, revises it while
still a draft, and publishes it with an explicit effective time. Another
supervisor can prepare the next draft only after the previous draft is
published or withdrawn. Withdrawing an old version prevents future starts
from selecting it, while every started run continues using the exact snapshot
it recorded at start.

This path crosses HTTP protocol handling, application orchestration, version
resolution, domain validation, and snapshot storage.

### 3. Vessel Run Lifecycle

Public entry points are:

- `POST /v1/vessels`
- `POST /v1/runs`
- `POST /v1/runs/{id}/reserve`
- `POST /v1/runs/{id}/start`
- `GET /v1/runs/{id}`

The client registers a vessel, creates a planned run against an approved
formula, reserves the vessel, starts fermentation, and reads the run. Capacity
conflicts, an unavailable vessel, duplicate run codes, and stale versions
produce stable failures without changing vessel or run state.

This path crosses HTTP protocol handling, application orchestration, domain
rules, storage coordination, and resource lifecycle.

### 4. Observations And Completion

Public entry points are:

- `POST /v1/runs/{id}/observations`
- `POST /v1/runs/{id}/conditioning`
- `POST /v1/runs/{id}/complete`
- `GET /v1/runs/{id}`

The client appends three ordered observations, starts conditioning, completes
the run, and reads the calculated result. Non-monotonic times, rising gravity,
out-of-range values, excessive sequences, incomplete data, and repeated
transitions return stable failures without changing persisted state.

This path crosses HTTP protocol handling, application orchestration, numeric
domain rules, and snapshot storage.

## Modules

- `cmd/fermentctl`: executable entry point for the HTTP service and bounded
  workflow checks.
- `internal/config`: environment and command-line configuration.
- `internal/clock`: deterministic clock boundary for production and checks.
- `internal/domain`: entities, validation, state transitions, and metrics.
- `internal/service`: use-case orchestration and revision coordination.
- `internal/storage`: snapshot repository, atomic replacement, and locking.
- `internal/httpapi`: routing, strict JSON decoding, error mapping, and health.
- `internal/workflowcheck`: real HTTP workflow checks used by the executable.

The dependency direction is:

`cmd -> workflowcheck/httpapi -> service -> domain`

`service -> storage`

`storage -> domain`

The domain package does not import outer packages.

## API Behavior

All business routes are JSON under `/v1`. A successful write returns the
updated aggregate and its version. A failed write returns:

```json
{
  "error": {
    "code": "stable_machine_code",
    "detail": "human-readable explanation"
  }
}
```

Write requests may include `X-Expected-Version`. When present, the service
rejects a stale write instead of overwriting newer state.

`GET /healthz` returns process health and the current snapshot revision.

## Persistence And Concurrency

The configured data directory contains `state.json`. Writes are serialized by
an in-process mutex. The complete state is encoded, committed to durable
storage, and then atomically renamed over the target. An update is not
published in memory unless persistence succeeds.

Reads use copied values and never expose repository-owned maps or slices. The
default address is `127.0.0.1:8088`, the request body limit is 1 MiB, and all
workflow checks use temporary directories.

## Validation Plan

The baseline declares `testing: deferred` and intentionally contains no unit
tests, fixtures, integration tests, or browser E2E files. A later verification
task owns systematic tests for validation boundaries, state transitions,
numeric calculations, stale revisions, storage failures, and concurrent
reservation attempts.

The current build command is `go build ./...`. Three bounded production checks
start the real HTTP handler, issue real JSON requests, check success and failure
behavior, and exit without external services:

- `go run ./cmd/fermentctl check formula-approval`
- `go run ./cmd/fermentctl check vessel-run-lifecycle`
- `go run ./cmd/fermentctl check observations-completion`
- `go run ./cmd/fermentctl check process-configuration-versioning`

## Scope Limits

This service does not control physical equipment, connect to a remote identity
provider, send notifications, expose a browser interface, replicate data, or
replace laboratory measurements. The API is intended for a trusted local
network or an internal adapter.
