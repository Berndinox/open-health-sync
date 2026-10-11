// SPDX-License-Identifier: AGPL-3.0-only

package model

import "strings"

// Source attribution: which upstream actually produced a sample, as opposed to which
// integration delivered it.
//
// An aggregator is a fan-in: one integration, many upstream vendors' devices. That
// breaks the assumption that "provider" identifies the sensor, and it breaks it in
// two ways that both cost data quality:
//
//   - Fusion ranks by provider, so every upstream behind one aggregator collapses
//     into a single priority slot.
//   - Dedup is provider-scoped, so the SAME physical sample arriving both directly
//     from a vendor and again through an aggregator hashes twice and survives twice.
//
// The fix is two identifiers instead of one:
//
//	provider    the integration: who holds the token, who to re-pull from.
//	sourceSlug  the sensor: whose hardware produced the sample.
//
// For a direct integration the two are equal.
//
// Like unit canonicalization, attribution is applied at ONE pipeline choke point
// rather than in each mapper: a rule enforced per mapper is a rule a new provider
// silently opts out of.

// Slug normalizes any spelling of a provider name to its canonical slug: trimmed
// and lowercased. Unknown names are returned that way rather than rejected: an
// unrecognized provider is still data that must flow, so normalizing is never a gate.
func Slug(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// ResolveSourceSlug returns the canonical slug of the upstream that actually produced
// a sample. upstream is the aggregator's own device or vendor attribution, verbatim
// from its payload; it is empty for direct integrations.
//
// Resolution is deliberately permissive: an upstream vendor with no dedicated mapper
// still gets a normalized slug rather than being dropped or folded into the
// aggregator, because attribution is worth keeping even for a source not modelled
// yet. Only an empty upstream falls back to the provider.
func ResolveSourceSlug(provider, upstream string) string {
	up := strings.ToLower(strings.TrimSpace(upstream))
	if up == "" {
		return Slug(provider)
	}
	// Normalize free-text vendor names ("Fitbit Charge 6") into a slug-shaped key.
	return slugify(up)
}

// slugify reduces a free-text vendor or device label to a slug-shaped key: lowercase,
// with every run of non-alphanumeric characters collapsed to a single underscore.
func slugify(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	lastUnderscore := false
	for _, r := range strings.ToLower(s) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastUnderscore = false
		default:
			if !lastUnderscore && b.Len() > 0 {
				b.WriteByte('_')
				lastUnderscore = true
			}
		}
	}
	return strings.TrimRight(b.String(), "_")
}

// upstreamFields are the keys an aggregator payload might carry its device or vendor
// attribution under. The snake_case spelling is what the collectors write; the
// camelCase and vendor spellings cover raw provider payloads that reach the ETL
// without passing through a normalizer (device uploads, replayed files).
var upstreamFields = []string{
	"source_slug",
	"sourceSlug",
	"upstream_provider",
	"upstreamProvider",
	"source_provider",
	"sourceProvider",
}

// UpstreamFromRecord extracts an aggregator's upstream attribution from a raw record.
// Returns empty when the record carries none, which is the normal case for a direct
// integration.
func UpstreamFromRecord(record map[string]any) string {
	for _, k := range upstreamFields {
		if v, ok := record[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// CanonicalizeSourceAttribution fills in the source attribution for every quantity
// sample in the result, so the source_slug column is never written empty and never
// disagrees with the provider vocabulary.
//
// A sample whose mapper already set SourceSlug is left alone: a mapper that can read
// per-record attribution out of its own payload knows better than the file-level
// fallback. Everything else resolves from the record's upstream field, and failing
// that from the provider itself.
func CanonicalizeSourceAttribution(mr *MapResult, record map[string]any, provider string) {
	if mr == nil || len(mr.QuantitySamples) == 0 {
		return
	}
	upstream := UpstreamFromRecord(record)
	fallback := ResolveSourceSlug(provider, upstream)

	for i := range mr.QuantitySamples {
		s := &mr.QuantitySamples[i]
		if s.SourceSlug != "" {
			s.SourceSlug = Slug(s.SourceSlug)
			continue
		}
		s.SourceSlug = fallback
	}
}
