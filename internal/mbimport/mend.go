package mbimport

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/adirkuhn/moneyharpy/internal/resource"
)

// A record can come in without part of it: a file Moneybird could not
// hand over or that was not taken here, the PDF Moneybird sent, or contact
// fields that did not pass. Importing again brings in what it came in
// without, once it can, and forgets the note of it; the rest of the record
// stays as it is. What only tells how a record came in, such as a customer
// number it could not keep, is no part to bring in and stays noted.

// mending is the record being mended: the notes of what it came in
// without, and which of them still hold.
type mending struct {
	open map[string]bool
	held map[string]bool
}

// mendable tells a note of a part that importing again may bring in.
func mendable(message string) bool {
	switch {
	case strings.HasPrefix(message, "file "), strings.Contains(message, "/download_pdf left out"):
		return true
	case strings.HasPrefix(message, "came in without its link"):
		return false
	}
	return strings.HasPrefix(message, "came in without ")
}

// partials returns, per record of a kind, the notes of parts it came in
// without that importing again may bring in, loaded once per kind.
func (r *run) partials(ctx context.Context, kind string) (map[resource.ID][]string, error) {
	if got, ok := r.partialNotes[kind]; ok {
		return got, nil
	}
	all, err := r.s.store.Partials(ctx, r.job.AdministrationID, kind)
	if err != nil {
		return nil, err
	}
	out := map[resource.ID][]string{}
	for id, messages := range all {
		for _, m := range messages {
			if mendable(m) {
				out[id] = append(out[id], m)
			}
		}
	}
	if r.partialNotes == nil {
		r.partialNotes = map[string]map[resource.ID][]string{}
	}
	r.partialNotes[kind] = out
	return out, nil
}

// mend runs a record that came in before through its import again, which
// brings in only what it came in without; then it forgets the notes that
// no longer hold. A part that still cannot come in keeps its note.
func (r *run) mend(ctx context.Context, kind string, id resource.ID, raw json.RawMessage, notes []string, one importOne) error {
	m := &mending{open: map[string]bool{}, held: map[string]bool{}}
	for _, n := range notes {
		m.open[n] = true
	}
	r.mending = m
	_, err := one(ctx, raw)
	r.mending = nil
	if err != nil {
		if isRecordProblem(err) || isFileProblem(err) {
			// The record is here; its notes stay as they were.
			return nil
		}
		return err
	}
	for n := range m.open {
		if !m.held[n] {
			if err := r.s.store.ForgetProblem(ctx, r.job.AdministrationID, kind, id, n); err != nil {
				return err
			}
		}
	}
	delete(r.partialNotes[kind], id)
	return nil
}

// wants tells whether a part whose note starts so is to come in: always on
// a first import, and while mending only when the record came in without it.
func (r *run) wants(prefix string) bool {
	if r.mending == nil {
		return true
	}
	for n := range r.mending.open {
		if strings.HasPrefix(n, prefix) {
			return true
		}
	}
	return false
}

// leftOut returns, while mending, the contact fields the record came in
// without, from its notes: "came in without a, b (why)".
func (r *run) leftOut() []string {
	var out []string
	for n := range r.mending.open {
		rest, ok := strings.CutPrefix(n, "came in without ")
		if !ok {
			continue
		}
		fields, _, _ := strings.Cut(rest, " (")
		for _, f := range strings.Split(fields, ", ") {
			if f = strings.TrimSpace(f); f != "" {
				out = append(out, f)
			}
		}
	}
	return out
}
