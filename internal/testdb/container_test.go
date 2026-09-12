package testdb

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestContainerExists_emptyName_isAbsentNotAnError asserts a caller that
// could not derive a name gets a plain "absent" rather than an error it
// would have to special-case: the anchored filter for an empty name matches
// nothing, which is exactly the outcome a name-less caller needs.
func TestContainerExists_emptyName_isAbsentNotAnError(t *testing.T) {
	t.Parallel()

	exists, err := ContainerExists(context.Background(), "")
	if err != nil {
		t.Fatalf("ContainerExists with an empty name: %v", err)
	}
	if exists {
		t.Errorf("ContainerExists(\"\") = true, want false")
	}
}

// TestContainerExists_unknownName_reportsAbsent asserts the lookup reaches
// the container runtime and comes back clean for a name nothing carries.
func TestContainerExists_unknownName_reportsAbsent(t *testing.T) {
	t.Parallel()

	name := fmt.Sprintf("lab-game-no-such-container-%d", os.Getpid())
	exists, err := ContainerExists(context.Background(), name)
	if err != nil {
		t.Fatalf("ContainerExists(%q): %v", name, err)
	}
	if exists {
		t.Errorf("ContainerExists(%q) = true, want false for a name nothing carries", name)
	}
}

// TestContainerExists_followsAServersLifetime asserts the positive answer
// against a container this test actually owns, and the negative one after
// it is removed. This is the pair the shared server's teardown depends on:
// it removes a container precisely when this function says one is there,
// so a lookup that always answered false would make the teardown a no-op
// and a lookup that always answered true would have it create a container
// in order to delete it. While the container is up, it also asserts that a
// strict substring of the container's own name is reported absent: two
// checkouts whose base names nest (one is a prefix of the other) must never
// have one's --down reattach to and remove the other's server.
func TestContainerExists_followsAServersLifetime(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	// Named per test process, so two binaries running at once on one host
	// never address each other's container.
	name := fmt.Sprintf("lab-game-exists-probe-%d", os.Getpid())
	substringOfName := strings.TrimPrefix(name, "lab-")

	server, err := StartServer(ctx, ServerOptions{ContainerName: name})
	if err != nil {
		t.Fatalf("StartServer(%q): %v", name, err)
	}
	stopped := false
	t.Cleanup(func() {
		if stopped {
			return
		}
		if err := server.Stop(context.Background()); err != nil {
			t.Errorf("stopping the server: %v", err)
		}
	})

	exists, err := ContainerExists(ctx, name)
	if err != nil {
		t.Fatalf("ContainerExists(%q) while the server runs: %v", name, err)
	}
	if !exists {
		t.Fatalf("ContainerExists(%q) = false while its container is running", name)
	}

	exists, err = ContainerExists(ctx, substringOfName)
	if err != nil {
		t.Fatalf("ContainerExists(%q) while %q runs: %v", substringOfName, name, err)
	}
	if exists {
		t.Errorf("ContainerExists(%q) = true while only %q is running; the match must be anchored", substringOfName, name)
	}

	if err := server.Stop(ctx); err != nil {
		t.Fatalf("stopping the server: %v", err)
	}
	stopped = true

	exists, err = ContainerExists(ctx, name)
	if err != nil {
		t.Fatalf("ContainerExists(%q) after removal: %v", name, err)
	}
	if exists {
		t.Errorf("ContainerExists(%q) = true after its container was removed", name)
	}
}
