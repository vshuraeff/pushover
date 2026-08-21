//go:build darwin

package notify

import (
	"context"
	"os/exec"
)

var runCommand = func(ctx context.Context, name string, args ...string) error {
	return exec.CommandContext(ctx, name, args...).Run()
}
