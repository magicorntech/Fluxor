package mongo

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"github.com/magicorntech/Fluxor/internal/store"
	"github.com/magicorntech/Fluxor/internal/store/storetest"
)

// These tests need a running MongoDB; set FLUXOR_TEST_MONGO_URI (e.g. mongodb://localhost:27017).
// Every test gets its own throw-away database that is dropped afterwards.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	uri := os.Getenv("FLUXOR_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("FLUXOR_TEST_MONGO_URI not set; skipping MongoDB tests")
	}
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	s, err := Connect(ctx, uri, "fluxor_test_"+hex.EncodeToString(b))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = s.DropDatabase(ctx)
		_ = s.Close(ctx)
	})
	return s
}

func TestConformance(t *testing.T) {
	storetest.Run(t, func(t *testing.T) store.Store { return newTestStore(t) })
}

func TestConnectFailsFast(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// Port 1 refuses connections; Connect must return an error instead of hanging.
	if _, err := Connect(ctx, "mongodb://127.0.0.1:1/?serverSelectionTimeoutMS=500", "x"); err == nil {
		t.Fatal("expected an error for an unreachable server")
	}
}
