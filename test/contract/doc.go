// SPDX-License-Identifier: AGPL-3.0-only

// Package contract checks the running server against api/openapi.yaml: the
// quickstart calls go through a recording client and every request and
// response is validated with kin-openapi. Run it with
// `go test -tags integration ./test/contract/` (make contract).
package contract
