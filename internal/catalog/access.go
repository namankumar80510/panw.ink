// Package catalog is the public fiction catalogue and reader API
// (IMPLEMENTATION.md §3, §7.2, §9.3) plus the content-write helpers shared by
// fixtures and the publishing studio.
//
// Access is computed from timestamps at request time (§3.2 items 4–6): no
// scheduler, job or cache has to run for a chapter to become public.
package catalog

import "time"

// Access is the reader-facing state of an entry.
type Access string

// Access states (§4.4 EntryStatus).
const (
	AccessPublic      Access = "public"      // public_at has passed: free for everyone
	AccessEarly       Access = "early"       // early window: verified supporters only
	AccessUpcoming    Access = "upcoming"    // scheduled, nobody can read the body yet
	AccessUnavailable Access = "unavailable" // draft, archived, or no published revision
)

// Release is what access depends on.
type Release struct {
	Workflow     string // draft | scheduled | published | archived
	EarlyAt      *time.Time
	PublicAt     *time.Time
	HasPublished bool // published_revision_id is set
}

// StateAt returns the access state at now.
func StateAt(r Release, now time.Time) Access {
	if (r.Workflow != "scheduled" && r.Workflow != "published") || !r.HasPublished || r.PublicAt == nil {
		return AccessUnavailable
	}
	if !now.Before(*r.PublicAt) {
		return AccessPublic
	}
	if r.EarlyAt != nil && !now.Before(*r.EarlyAt) {
		return AccessEarly
	}
	return AccessUpcoming
}

// Listed reports whether a released entry appears in a public table of
// contents: everything except drafts/archived/unpublished.
func (a Access) Listed() bool { return a != AccessUnavailable }
