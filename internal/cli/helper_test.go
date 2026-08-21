package cli

import (
	"strings"
	"testing"
)

func TestHelperInstallRequiresSourceBundle(t *testing.T) {
	originalHelperBundlePath := HelperBundlePath
	HelperBundlePath = ""
	t.Cleanup(func() {
		HelperBundlePath = originalHelperBundlePath
	})

	_, err := executeCLI(t, nil, "helper", "install")
	if err == nil {
		t.Fatal("povr helper install error = nil, want missing source error")
	}
	if !strings.Contains(err.Error(), "--from") || !strings.Contains(err.Error(), "baked-in") {
		t.Errorf("povr helper install error = %q, want source guidance", err)
	}
}
