package testdb

import (
	"context"
	"fmt"
	"regexp"

	"github.com/moby/moby/client"
	"github.com/testcontainers/testcontainers-go"
)

// ContainerExists reports whether a container carrying exactly this name is
// present on the container runtime, in any state. A stopped container counts
// as present: it still holds the anonymous volume the image's VOLUME
// directive created, so a caller reclaiming the server's resources has to be
// able to tell "not running" apart from "not there". An empty name matches
// nothing and is reported as absent rather than as an error, so a caller that
// could not derive a name needs no special case.
func ContainerExists(ctx context.Context, name string) (bool, error) {
	if name == "" {
		return false, nil
	}

	cli, err := testcontainers.NewDockerClientWithOpts(ctx)
	if err != nil {
		return false, fmt.Errorf("container runtime client: %w", err)
	}
	defer func() { _ = cli.Close() }()

	// The runtime matches the name filter as a regular expression, so an
	// anchored, quoted name is what keeps one checkout's server from
	// answering for another whose name merely contains it.
	listed, err := cli.ContainerList(ctx, client.ContainerListOptions{
		All:     true,
		Filters: make(client.Filters).Add("name", "^"+regexp.QuoteMeta(name)+"$"),
	})
	if err != nil {
		return false, fmt.Errorf("list containers named %s: %w", name, err)
	}

	return len(listed.Items) > 0, nil
}
