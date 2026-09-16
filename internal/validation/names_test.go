package validation

import "testing"

func TestNameAcceptsSafeFilename(t *testing.T) {
	if got, err := Name("Quarterly report.pdf"); err != nil || got != "Quarterly report.pdf" {
		t.Fatalf("got %q, %v", got, err)
	}
}
func TestNameRejectsTraversalAndSeparators(t *testing.T) {
	for _, name := range []string{"", ".", "..", "../secret", `folder\\file`, "nul\x00name"} {
		if _, err := Name(name); err == nil {
			t.Fatalf("accepted unsafe name %q", name)
		}
	}
}
