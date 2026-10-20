package storetest

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/magicorntech/Fluxor/internal/domain"
	"github.com/magicorntech/Fluxor/internal/store"
)

func audit(t *testing.T, s store.Store) {
	ctx := context.Background()
	base := time.Date(2026, 5, 6, 7, 8, 9, 0, time.UTC)

	if got, err := s.ListAudit(ctx, store.AuditFilter{}); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("empty list must be a non-nil empty slice: %v %v", got, err)
	}

	ev := func(id string, at time.Time, actor, action string, result domain.AuditResult) domain.AuditEvent {
		return domain.AuditEvent{
			ID: id, Time: at, ActorID: actor, ActorName: "name-" + actor, Action: action, Subject: "cluster:c1", SubjectName: "alpha",
			Result: result, Status: 200, IP: "10.0.0.1", Details: map[string]string{"k": "v"},
		}
	}
	events := []domain.AuditEvent{
		ev("e1", base, "u1", "cluster.create", domain.AuditSuccess),
		ev("e2", base.Add(time.Second), "u2", "cluster.delete", domain.AuditDenied),
		ev("e3", base.Add(2*time.Second), "u1", "user.create", domain.AuditSuccess),
		ev("e4", base.Add(2*time.Second), "u1", "user.update", domain.AuditFailure), // same instant as e3
		ev("e5", base.Add(3*time.Second), "", "auth.login", domain.AuditDenied),
	}
	for _, e := range events {
		if err := s.AddAudit(ctx, e); err != nil {
			t.Fatalf("add %s: %v", e.ID, err)
		}
	}

	ids := func(f store.AuditFilter) []string {
		list, err := s.ListAudit(ctx, f)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		out := []string{}
		for _, e := range list {
			out = append(out, e.ID)
		}
		return out
	}
	eq := func(name string, got, want []string) {
		t.Helper()
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("%s: got %v, want %v", name, got, want)
		}
	}

	eq("newest first; events at the same instant by descending ID", ids(store.AuditFilter{}), []string{"e5", "e4", "e3", "e2", "e1"})
	eq("by actor", ids(store.AuditFilter{ActorID: "u1"}), []string{"e4", "e3", "e1"})
	eq("by action", ids(store.AuditFilter{Action: "user.create"}), []string{"e3"})
	eq("by result", ids(store.AuditFilter{Result: domain.AuditDenied}), []string{"e5", "e2"})
	eq("by actor and result", ids(store.AuditFilter{ActorID: "u1", Result: domain.AuditSuccess}), []string{"e3", "e1"})
	eq("from is inclusive", ids(store.AuditFilter{From: base.Add(2 * time.Second)}), []string{"e5", "e4", "e3"})
	eq("to is exclusive", ids(store.AuditFilter{To: base.Add(2 * time.Second)}), []string{"e2", "e1"})
	eq("a range", ids(store.AuditFilter{From: base.Add(time.Second), To: base.Add(3 * time.Second)}), []string{"e4", "e3", "e2"})
	eq("limit", ids(store.AuditFilter{Limit: 2}), []string{"e5", "e4"})

	// Paging walks the whole list exactly once, including across events at the same instant.
	var paged []string
	var cursor *domain.AuditCursor
	for i := 0; i < 10; i++ {
		page, err := s.ListAudit(ctx, store.AuditFilter{Limit: 2, Before: cursor})
		if err != nil {
			t.Fatal(err)
		}
		if len(page) == 0 {
			break
		}
		for _, e := range page {
			paged = append(paged, e.ID)
		}
		last := page[len(page)-1]
		cursor = &domain.AuditCursor{Time: last.Time, ID: last.ID}
	}
	eq("paging", paged, []string{"e5", "e4", "e3", "e2", "e1"})

	list, _ := s.ListAudit(ctx, store.AuditFilter{ActorID: "u2"})
	if len(list) != 1 {
		t.Fatalf("got %d events", len(list))
	}
	got := list[0]
	if got.ID != "e2" || got.ActorName != "name-u2" || got.Action != "cluster.delete" || got.Subject != "cluster:c1" || got.SubjectName != "alpha" ||
		got.Result != domain.AuditDenied || got.Status != 200 || got.IP != "10.0.0.1" || got.Details["k"] != "v" || !got.Time.Equal(base.Add(time.Second)) {
		t.Fatalf("round trip mismatch: %+v", got)
	}
	// An event without details comes back with an empty map, never nil.
	bare := ev("e6", base.Add(4*time.Second), "u3", "x", domain.AuditSuccess)
	bare.Details = nil
	if err := s.AddAudit(ctx, bare); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.ListAudit(ctx, store.AuditFilter{ActorID: "u3"}); len(list) != 1 || list[0].Details == nil {
		t.Fatalf("details must never be nil: %+v", list)
	}
}
