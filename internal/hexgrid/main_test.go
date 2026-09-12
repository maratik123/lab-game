package hexgrid_test

import (
	"os"
	"testing"

	"github.com/maratik123/lab-game/internal/leaktest"
)

func TestMain(m *testing.M) {
	os.Exit(leaktest.Main(m, (*testing.M).Run))
}
