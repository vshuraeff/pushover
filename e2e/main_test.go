//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

var binaryPath string

func TestMain(m *testing.M) {
	code := func() int {
		tempDir, err := os.MkdirTemp("", "povr-e2e-")
		if err != nil {
			fmt.Fprintf(os.Stderr, "create e2e build directory: %v\n", err)
			return 1
		}
		defer os.RemoveAll(tempDir)

		moduleRoot, err := filepath.Abs("..")
		if err != nil {
			fmt.Fprintf(os.Stderr, "resolve module root: %v\n", err)
			return 1
		}
		binaryPath = filepath.Join(tempDir, "povr")
		build := exec.Command("go", "build", "-o", binaryPath, "./cmd/povr")
		build.Dir = moduleRoot
		if output, err := build.CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "build povr: %v\n%s", err, output)
			return 1
		}
		return m.Run()
	}()
	os.Exit(code)
}
