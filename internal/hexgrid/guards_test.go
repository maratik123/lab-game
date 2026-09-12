package hexgrid_test

import (
	"testing"

	"github.com/maratik123/lab-game/internal/detguard"
	"github.com/maratik123/lab-game/internal/repotest"
)

func TestGuard_DeterminismPredicates(t *testing.T) {
	t.Parallel()
	dir := repotest.RootPath(t, "internal/hexgrid")
	if problems := detguard.Check(t, dir); len(problems) != 0 {
		t.Errorf("detguard.Check(hexgrid) problems:\n%v", problems)
	}
}
