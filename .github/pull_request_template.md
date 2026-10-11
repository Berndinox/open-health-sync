## What and why

<!-- What this changes and the problem it solves. Link the issue: Fixes #123 -->

## How it was tested

<!-- Commands you ran and what they showed. -->

## Checklist

- [ ] `go vet ./...` and `go test ./...` pass
- [ ] `make integration` passes when the change touches the database, the pipeline or the API
- [ ] Tests cover the change
- [ ] Docs updated (`docs/`, `api/openapi.yaml`, `CHANGELOG.md` under Unreleased)
- [ ] No real health data, keys or tokens in code, fixtures, logs or this description
- [ ] New Go files start with `// SPDX-License-Identifier: AGPL-3.0-only`
- [ ] Conventional commit messages (`feat(withings): ...`, `fix(etl): ...`)
