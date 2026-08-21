package cli

import (
	"strings"
	"testing"
)

func TestCompletionFish(t *testing.T) {
	output, err := executeCLI(t, nil, "completion", "fish")
	if err != nil {
		t.Fatalf("povr completion fish error = %v", err)
	}
	if output == "" {
		t.Fatal("povr completion fish output is empty")
	}
	if !strings.Contains(output, "complete -c povr") {
		t.Errorf("povr completion fish output missing %q marker, got:\n%s", "complete -c povr", output)
	}
	for _, marker := range []string{"__povr_perform_completion", "# fish completion for povr"} {
		if !strings.Contains(output, marker) {
			t.Errorf("povr completion fish output missing marker %q", marker)
		}
	}
}

func TestCompletionUnsupportedArg(t *testing.T) {
	_, err := executeCLI(t, nil, "completion", "fish", "extra")
	if err == nil {
		t.Fatal("povr completion fish extra error = nil, want error")
	}
}
