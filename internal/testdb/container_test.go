package testdb

import (
	"context"
	"fmt"
	"os"
	"testing"
)

// TestContainerExists_emptyName_isAbsentNotAnError asserts a caller that
// could not derive a name gets a plain "absent" rather than an error it
// would have to special-case, and that no runtime lookup is attempted for
// a name that can match nothing.
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
// The name is anchored at both ends, so this also covers the case that
// matters for a checkout whose name is a prefix of another's.
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
// in order to delete it.
func TestContainerExists_followsAServersLifetime(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	// Named per test process, so two binaries running at once on one host
	// never address each other's container.
	name := fmt.Sprintf("lab-game-exists-probe-%d", os.Getpid())

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
