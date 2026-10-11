// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"net/http"

	"github.com/soma-intelligence/open-health-sync/internal/connector"
)

type providerJSON struct {
	Slug string `json:"slug"`
	// Kinds lists what the connector does: oauth (the server pulls), push (the
	// provider notifies), device (an SDK uploads).
	Kinds []string `json:"kinds"`
}

func (s *server) mountProviders(mux *http.ServeMux) {
	s.tenantRoute(mux, "GET /v1/providers", s.listProviders)
}

func (s *server) listProviders(w http.ResponseWriter, r *http.Request) {
	out := make([]providerJSON, 0, len(s.d.Providers))
	for _, c := range s.d.Providers {
		p := providerJSON{Slug: c.Slug(), Kinds: []string{}}
		if _, ok := connector.AsOAuth(c); ok {
			p.Kinds = append(p.Kinds, "oauth")
		}
		if _, ok := connector.AsPush(c); ok {
			p.Kinds = append(p.Kinds, "push")
		}
		if _, ok := connector.AsDevice(c); ok {
			p.Kinds = append(p.Kinds, "device")
		}
		out = append(out, p)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}
