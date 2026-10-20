package domain

import "testing"

func validBinding() Binding {
	return Binding{
		Subject: Subject{Kind: SubjectUser, ID: "u1"},
		RoleID:  "viewer",
		Scope:   AnyScope(),
	}
}

func TestBindingValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(b *Binding)
		wantErr bool
	}{
		{"ok", func(b *Binding) {}, false},
		{"team subject", func(b *Binding) { b.Subject.Kind = SubjectTeam }, false},
		{"cluster by id", func(b *Binding) { b.Scope.Cluster = Ref{Kind: RefID, ID: "c1"} }, false},
		{"cluster by group", func(b *Binding) { b.Scope.Cluster = Ref{Kind: RefGroup, ID: "g1"} }, false},
		{"bad subject kind", func(b *Binding) { b.Subject.Kind = "robot" }, true},
		{"empty subject id", func(b *Binding) { b.Subject.ID = "" }, true},
		{"empty role", func(b *Binding) { b.RoleID = "" }, true},
		{"empty ref kind", func(b *Binding) { b.Scope.Cluster = Ref{} }, true},
		{"any with id", func(b *Binding) { b.Scope.Application = Ref{Kind: RefAny, ID: "x"} }, true},
		{"id without id", func(b *Binding) { b.Scope.Cluster = Ref{Kind: RefID} }, true},
		{"group without id", func(b *Binding) { b.Scope.Application = Ref{Kind: RefGroup} }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := validBinding()
			tt.mutate(&b)
			if err := b.Validate(); (err != nil) != tt.wantErr {
				t.Fatalf("Validate() = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBindingKeyDistinguishesGrants(t *testing.T) {
	a := validBinding()
	same := validBinding()
	same.ID, same.CreatedBy = "other-id", "someone"
	if a.Key() != same.Key() {
		t.Fatal("id and audit fields must not affect the key")
	}

	variants := map[string]func(b *Binding){
		"subject":     func(b *Binding) { b.Subject.ID = "u2" },
		"subjectKind": func(b *Binding) { b.Subject.Kind = SubjectTeam },
		"role":        func(b *Binding) { b.RoleID = "deployer" },
		"cluster":     func(b *Binding) { b.Scope.Cluster = Ref{Kind: RefID, ID: "c1"} },
		"clusterKind": func(b *Binding) { b.Scope.Cluster = Ref{Kind: RefGroup, ID: "c1"} },
		"application": func(b *Binding) { b.Scope.Application = Ref{Kind: RefID, ID: "a1"} },
	}
	for name, mutate := range variants {
		b := validBinding()
		mutate(&b)
		if b.Key() == a.Key() {
			t.Errorf("changing the %s must change the key", name)
		}
	}
}

func TestScopeIsAny(t *testing.T) {
	if !AnyScope().IsAny() {
		t.Fatal("AnyScope must be any")
	}
	s := AnyScope()
	s.Cluster = Ref{Kind: RefID, ID: "c"}
	if s.IsAny() {
		t.Fatal("a narrowed scope is not any")
	}
}
