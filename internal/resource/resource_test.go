package resource

import (
	"sync"
	"testing"
	"time"
)

func TestGeneratorNextIsUniqueAndIncreasing(t *testing.T) {
	g := NewGenerator(1)
	const n = 10000
	var mu sync.Mutex
	seen := make(map[ID]bool, n)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range n / 8 {
				id := g.Next()
				mu.Lock()
				if seen[id] {
					t.Errorf("Next() = %d, already generated", id)
				}
				seen[id] = true
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	first, second := g.Next(), g.Next()
	if second <= first {
		t.Errorf("Next() = %d after %d, want increasing", second, first)
	}
}

func TestGeneratorSurvivesClockMovingBackwards(t *testing.T) {
	now := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	g := NewGenerator(1)
	g.now = func() time.Time { return now }
	first := g.Next()

	now = now.Add(-time.Hour)
	if second := g.Next(); second <= first {
		t.Errorf("Next() = %d after clock rewind, want > %d", second, first)
	}
}

func TestParseID(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    ID
		wantErr bool
	}{
		{name: "large integer", in: "416072101932303822", want: 416072101932303822},
		{name: "empty", in: "", wantErr: true},
		{name: "letters", in: "abc", wantErr: true},
		{name: "negative", in: "-5", wantErr: true},
		{name: "zero", in: "0", wantErr: true},
		{name: "sign prefix", in: "+5", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseID(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseID(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ParseID(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestURI(t *testing.T) {
	tests := []struct {
		name string
		ref  Ref
		want string
	}{
		{
			name: "administration is its own root",
			ref:  Ref{ID: 42, Type: TypeAdministration, AdministrationID: 42},
			want: "/42",
		},
		{
			name: "scoped resource nests under its administration",
			ref:  Ref{ID: 7, Type: Type("contacts"), AdministrationID: 42},
			want: "/42/contacts/7",
		},
		{
			name: "identity nests under its administration",
			ref:  Ref{ID: 7, Type: TypeIdentity, AdministrationID: 42},
			want: "/42/identities/7",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ref.URI(); got != tt.want {
				t.Errorf("%+v.URI() = %q, want %q", tt.ref, got, tt.want)
			}
		})
	}
}

func TestPermalink(t *testing.T) {
	if got, want := Permalink(99), "/r/99"; got != want {
		t.Errorf("Permalink(99) = %q, want %q", got, want)
	}
}
