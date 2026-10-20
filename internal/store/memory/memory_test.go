package memory

import (
	"testing"

	"github.com/magicorntech/Fluxor/internal/store"
	"github.com/magicorntech/Fluxor/internal/store/storetest"
)

func TestConformance(t *testing.T) {
	storetest.Run(t, func(t *testing.T) store.Store { return New() })
}
