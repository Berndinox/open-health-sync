# Apple Health connector

Slug `apple_health`. A device connector: an iOS app (yours; no SDK ships yet) reads HealthKit
and uploads the data, the server never talks to Apple. It needs no operator keys and is always enabled.

## Upload contract

`POST /v1/ingest/apple-health` stores one **part**: a gzip compressed NDJSON file.

| | |
|---|---|
| Auth | `Authorization: Bearer <user token>` with the `upload` scope |
| Body | gzip of NDJSON: one JSON object per line, at least one line, each line at most 32 MiB uncompressed |
| `Content-Type` | not checked; send `application/gzip` |
| Size | at most 64 MiB of gzip bytes per part, 413 above it. A part is never truncated. A part that decompresses to more than 512 MiB is also refused with 413. |

Headers, all required:

| Header | Value |
|---|---|
| `X-OHS-Type` | HealthKit type identifier of every line of the part, for example `HKQuantityTypeIdentifierHeartRate`. Letters, digits, `_` and `-`, at most 128. |
| `X-OHS-Day` | The day the part belongs to, `YYYY-MM-DD`. It is part of the raw key. |
| `X-OHS-Batch` | An id for the upload run, letters, digits, `_` and `-`, at most 128. |
| `X-OHS-Part` | Integer from 0 to 1000000, the part number in the batch. |

A part holds one data type. To upload more, send more parts.

**Keys and duplicates.** The raw object is stored at
`raw/<tenant>/<user>/apple_health/<day>/<batch>_<part>.ndjson.gz`. The type is recorded with the
object but is not part of the key, so **`(batch, part)` must be unique across all the types you
upload for one day**: two types sent as the same batch and part collide. Re-sending the same
bytes is safe and answers 200 with `"duplicate": true`. Different bytes under a key that is
already taken answer 409; a raw file is never rewritten. Use a fresh batch id for new data.

Responses:

| Status | Meaning |
|---|---|
| 200 | `{"key": "raw/...", "duplicate": false}`: stored and queued for processing |
| 400 | A header is missing or invalid (the message names it), the body is empty, not gzip, truncated, or a line is not a JSON object |
| 401, 403 | Missing or expired token, a token without the `upload` scope, or a user being deleted (their tokens stop working at once) |
| 409 | Other bytes are already stored under this day, batch and part, whatever their type: batch and part must be unique across types |
| 410 | The user started being deleted while this upload was being stored (its token was accepted just before); nothing is kept |
| 413 | Over 64 MiB of gzip, or over 512 MiB decompressed |
| 503 | Too many uploads in progress on this instance; `Retry-After` says when to retry |

Processing is asynchronous. Poll `GET /v1/users/{id}/coverage` until `pending_jobs` is 0, then
read the data (see [api.md](../api.md#knowing-when-an-upload-is-processed)).

## Line formats

Each line is routed by the fields it carries, not by `X-OHS-Type`.

Quantity samples:

```json
{"quantityType":"HKQuantityTypeIdentifierHeartRate","startDate":"2026-03-01T14:30:00Z","endDate":"2026-03-01T14:30:00Z","value":72.5,"unit":"count/min","sourceName":"Apple Watch","sourceBundleId":"com.apple.health.ABC","device":"Apple Watch","deviceModel":"Watch","productType":"Watch7,1","wasUserEntered":false,"metadata":{"HKTimeZone":"Europe/Paris"}}
```

`value` and `unit` are what HealthKit reports; the server converts to the storage unit (here
`count/min` becomes `bpm`, a 0.97 oxygen saturation fraction becomes 97 percent). The
`HKTimeZone` metadata key is kept as the sample's `timezone` and decides its local day.
A line of a quantity or category type the server does not know, or a quantity line without
`value`, is skipped without an error: nothing is stored for it and it does not appear in
coverage. The server counts unknown types and logs each one once (`apple_health: unknown type
skipped`, with the HealthKit identifier), so a missing mapping shows in the logs.

Other line kinds are recognized by their discriminator field:

| Field | Becomes |
|---|---|
| `quantityType` (dietary identifiers) | nutrition logs |
| `categoryType` | category samples; `HKCategoryTypeIdentifierSleepAnalysis` makes sleep sessions and phases |
| `workoutActivityType` | workouts |
| `correlationType` | blood pressure pairs, food entries |
| `classification` | ECG recording metadata |
| `timeSinceSampleStart` and `value` | ECG voltage samples |
| `heartbeats` | beat to beat RR intervals (see below) |
| `latitude` and `longitude` | workout route points |

ECG recordings, ECG voltage samples and heartbeat intervals are stored, but no read route serves
them yet (planned). Clinical record and FHIR resource lines are accepted and dropped on purpose.

### Heartbeat series

HealthKit gives beat times, not intervals, so a series is one line carrying all its beats:

```json
{"startDate":"2026-03-01T14:30:00Z","uuid":"series-1","heartbeats":[{"timeSinceSeriesStart":0.0,"precededByGap":false},{"timeSinceSeriesStart":0.82,"precededByGap":false},{"timeSinceSeriesStart":1.65,"precededByGap":true}]}
```

Each beat after the first yields one RR interval in milliseconds, stamped at the beat's own
time. A beat with `precededByGap: true` follows missing data, so no interval is produced for
it. A beat that is not after the previous kept one is dropped. A line with a single beat and no
`heartbeats` array is rejected, because one beat gives no interval.

### Example files

Ready to upload, one NDJSON line per record:

- `connectors/applehealth/testdata/e2e/`: a full day (2026-01-10) of heart rate, steps, sleep,
  workouts and SpO2, as the end to end suite uploads it. Each file is one part, named
  `<part>_<type>.ndjson`: gzip it and send it with `X-OHS-Part` and `X-OHS-Type` taken from the
  name, `X-OHS-Day: 2026-01-10` and one `X-OHS-Batch` for the whole set.
- `connectors/applehealth/testdata/ios_upload_lines.ndjson`: the lines an iOS client sends,
  several kinds mixed (mixed lines in one part are accepted).

The other files in `connectors/applehealth/testdata/` are unit test inputs, one record per file
and pretty printed (`mixed_types.json` is a JSON array), so they are not upload ready as they
are; `jq -c 'if type == "array" then .[] else . end' FILE` turns one into NDJSON. Some are
invalid on purpose: `bad_values.json` (a value that is not a number) and `missing_fields.json`
(no `startDate`) are rejected and counted in `rejected_records`, and `unknown_type.json` and
the unknown line of `mixed_types.json` are skipped.

## Processing notes

- Content is de-duplicated by hash, so the same reading uploaded twice is stored once.
- A running total HealthKit re-sends with a newer value (a day's step count) replaces the stored
  one.
- Daily step, energy, distance and flight totals are derived per local day from samples. When an
  iPhone and an Apple Watch both report, the larger source total wins instead of being added.
- A value outside its hard plausible range rejects the record; outside the soft range it is
  stored and flagged in its metadata.
