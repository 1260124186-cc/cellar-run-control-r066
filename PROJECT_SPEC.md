# Cellar Run Control Specification

## Goal

Cellar Run Control is a local Go HTTP service for a small production cellar.
It coordinates approved formulas, versioned process profiles, vessel
reservation, active fermentation runs, gravity observations, conditioning,
completion, and vessel release.

The project is a healthy initialization baseline. It contains no intentional
defects, no model traces, and no automated tests. Verification is intentionally
deferred to a later engineering task.

## Users

- Production planner: creates formulas, updates process steps, selects a
  process profile, and approves a formula before it can be used.
- Cellar supervisor: drafts, publishes, revises, and withdraws versioned
  process profiles that carry seasonal and style-specific expectations.
- Cellar operator: registers vessels, creates runs, reserves a vessel, and
  starts fermentation.
- Fermentation observer: appends timestamped gravity observations and moves a
  run through conditioning and completion.
- Integration client: calls the stable JSON API from a local production script.

## Entities

### ProcessProfile

A process profile is one version of a named profile family. It contains an
identifier, normalized family name, publication version, expected fermentation
temperature band, expected fermentation duration window in days, observation
thresholds for conditioning and completion, an effective time, lifecycle
state, an optimistic-concurrency revision, and timestamps.

States are `draft`, `published`, and `withdrawn`.

- Family names are unique after case folding and whitespace normalization; a
  new version of an existing family is created by revising a published
  version, never by reusing the name.
- The temperature band stays within -5 to 60 Celsius and the minimum cannot
  exceed the maximum.
- The duration window stays within 0 to 120 days and the minimum cannot
  exceed the maximum.
- Observation thresholds stay within 1 to 500 and the conditioning threshold
  cannot exceed the completion threshold.
- Only a draft can be edited or published; publishing freezes the version.
- A published version is immutable. Changing expectations requires revising
  it into a new draft with the next publication version.
- Withdrawing a published version stops new runs from binding to it but never
  rewrites runs that already bound it.

### Formula

A formula contains an identifier, normalized name, style label, selected
process profile family name, target original gravity, target final gravity,
maximum fermentation temperature, minimum and maximum planned days, ordered
process steps, lifecycle state, version, and timestamps.

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
- A draft formula may select or change its process profile family.
- `draft -> approved` requires a complete valid step list and a selected
  process profile family that has at least one published version.
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
an optional vessel identifier, planned volume, notes, lifecycle state, a
profile binding, timetable, ordered observations, final metrics, version, and
timestamps.

States are `planned`, `reserved`, `fermenting`, `conditioning`, `completed`,
and `aborted`.

- Planned volume is greater than zero and cannot exceed vessel capacity.
- Only an approved formula may be attached to a run.
- `planned -> reserved` atomically reserves an available vessel.
- `reserved -> fermenting` requires that the vessel still belongs to the run
  and that the formula's process profile family has a published version whose
  effective time has passed. Starting binds that version to the run and
  records the version, the binding time, the reason, and a parameter snapshot.
- `fermenting -> conditioning` requires at least as many observations as the
  bound profile version demands.
- `conditioning -> completed` requires the bound profile's observation
  threshold, an elapsed fermentation time inside the bound duration window,
  and a final observation temperature inside the bound temperature band.
- Completion computes apparent attenuation and estimated alcohol by volume from
  the formula target original gravity and the last observation.
- Completion moves the vessel to cleaning without losing its run association.
- A non-completed run can be aborted. Aborting before fermentation releases an
  available vessel; aborting after fermentation schedules the vessel for
  cleaning.
- Completed and aborted runs are immutable.
- A run can carry at most 500 observations.
- A bound profile version keeps judging its run even after the version is
  withdrawn or superseded; the binding record explains which version applies
  and why.

### GravityObservation

An observation contains a run identifier, contiguous sequence number, observed
time, gravity, temperature, optional note, and receipt time.

- Gravity is in the inclusive range 0.980 to 1.200.
- Temperature is in the inclusive range -5 to 60 Celsius.
- Observed time begins at or after the run start.
- Observation times strictly increase within a run.
- Gravity must not rise more than 0.010 above the immediately preceding value.
- Observation sequences are contiguous and start at 1.

## Connected Workflows

### 1. Formula Approval

Public entry points are:

- `POST /v1/formulas`
- `PUT /v1/formulas/{id}/steps`
- `PUT /v1/formulas/{id}/profile`
- `POST /v1/formulas/{id}/approve`
- `GET /v1/formulas/{id}`

The client creates a draft, sets its process step list atomically, selects a
process profile family, approves it, and reads the approved record. Invalid
ranges, duplicate names, empty steps, a missing or unpublished profile, and
repeated approval return stable error codes without partial writes.

This path crosses HTTP protocol handling, application orchestration, domain
rules, and snapshot storage.

### 2. Vessel Run Lifecycle

Public entry points are:

- `POST /v1/vessels`
- `POST /v1/runs`
- `POST /v1/runs/{id}/reserve`
- `POST /v1/runs/{id}/start`
- `GET /v1/runs/{id}`

The client registers a vessel, creates a planned run against an approved
formula, reserves the vessel, starts fermentation, and reads the run. Starting
resolves the formula's process profile family to the latest published version
effective at that moment and records the binding on the run. Capacity
conflicts, an unavailable vessel, duplicate run codes, a missing effective
profile version, and stale versions produce stable failures without changing
vessel or run state.

This path crosses HTTP protocol handling, application orchestration, domain
rules, storage coordination, and resource lifecycle.

### 3. Observations And Completion

Public entry points are:

- `POST /v1/runs/{id}/observations`
- `POST /v1/runs/{id}/conditioning`
- `POST /v1/runs/{id}/complete`
- `GET /v1/runs/{id}`

The client appends three ordered observations, starts conditioning, completes
the run, and reads the calculated result. Conditioning and completion are
judged by the profile version bound at run start, including its observation
thresholds, duration window, and temperature band. Non-monotonic times,
rising gravity, out-of-range values, excessive sequences, incomplete data,
profile violations, and repeated transitions return stable failures without
changing persisted state.

This path crosses HTTP protocol handling, application orchestration, numeric
domain rules, and snapshot storage.

### 4. Process Profile Versioning

Public entry points are:

- `POST /v1/profiles`
- `GET /v1/profiles`
- `GET /v1/profiles/{id}`
- `PUT /v1/profiles/{id}`
- `POST /v1/profiles/{id}/publish`
- `POST /v1/profiles/{id}/revise`
- `POST /v1/profiles/{id}/withdraw`

The client creates a draft profile family, edits its expectations, publishes
it, revises the published version into a successor draft, and withdraws
superseded versions. Duplicate family names, edits to published versions,
repeated publication, stale revisions, and invalid expectation ranges return
stable error codes without partial writes. Runs started before a republication
or withdrawal keep their originally bound version, and each run exposes which
version it adopted and why.

This path crosses HTTP protocol handling, application orchestration, domain
rules, optimistic concurrency, and snapshot storage.

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

The current build command is `go build ./...`. Four bounded production checks
start the real HTTP handler, issue real JSON requests, check success and failure
behavior, and exit without external services:

- `go run ./cmd/fermentctl check formula-approval`
- `go run ./cmd/fermentctl check vessel-run-lifecycle`
- `go run ./cmd/fermentctl check observations-completion`
- `go run ./cmd/fermentctl check profile-binding`

## Scope Limits

This service does not control physical equipment, connect to a remote identity
provider, send notifications, expose a browser interface, replicate data, or
replace laboratory measurements. The API is intended for a trusted local
network or an internal adapter.
