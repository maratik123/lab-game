package chat

import (
	"os"
	"testing"

	"github.com/maratik123/lab-game/internal/leaktest"
	"github.com/maratik123/lab-game/internal/testdb"
)

func TestMain(m *testing.M) {
	os.Exit(leaktest.Main(m, testdb.Main))
}
