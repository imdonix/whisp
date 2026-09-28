package inject

import (
	"testing"
	"time"
)

// The copy tool leaves a child serving the selection; copyTo must still return
// promptly, and what it wrote must come back out.
func TestCopyToReturns(t *testing.T) {
	clip, err := newClipboard()
	if err != nil {
		t.Skip(err)
	}
	in := &Injector{clip: clip}
	start := time.Now()
	if err := in.copyTo("whisp test", false); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Fatalf("copyTo took %v", d)
	}
	out, err := in.readClipboard()
	if err != nil || string(out) != "whisp test" {
		t.Fatalf("clipboard = %q, %v", out, err)
	}
}
