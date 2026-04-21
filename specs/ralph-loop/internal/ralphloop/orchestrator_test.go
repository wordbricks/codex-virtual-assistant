package ralphloop

import (
	"testing"
	"time"
)

func TestNormalizeTurnTimeoutsDefaultsAndCapsIdleTimeout(t *testing.T) {
	turnTimeout, idleTimeout := normalizeTurnTimeouts(0, 0)
	if turnTimeout != 2*time.Hour {
		t.Fatalf("turnTimeout = %s, want 2h", turnTimeout)
	}
	if idleTimeout != 10*time.Minute {
		t.Fatalf("idleTimeout = %s, want 10m", idleTimeout)
	}

	turnTimeout, idleTimeout = normalizeTurnTimeouts(120, 600)
	if turnTimeout != 2*time.Minute {
		t.Fatalf("turnTimeout = %s, want 2m", turnTimeout)
	}
	if idleTimeout != 2*time.Minute {
		t.Fatalf("idleTimeout = %s, want capped 2m", idleTimeout)
	}
}
