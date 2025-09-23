package latency

import (
	"testing"

	"github.com/relab/hotstuff"
)

func TestLatencySymmetry(t *testing.T) {
	for _, fromLoc := range allLocations {
		for _, toLoc := range allLocations {
			latency := Between(fromLoc, toLoc)
			reverse := Between(toLoc, fromLoc)
			if latency != reverse {
				t.Errorf("LatencyCity(%s, %s) != LatencyCity(%s, %s) ==> %v != %v", fromLoc, toLoc, toLoc, fromLoc, latency, reverse)
			}
		}
	}
	for i := range allLocations {
		for j := range allLocations {
			latency := allLatencies[i][j]
			reverse := allLatencies[j][i]
			if latency != reverse {
				t.Errorf("Latency(%d, %d) != Latency(%d, %d) ==> %v != %v", i, j, j, i, latency, reverse)
			}
		}
	}
}

func TestLatencyMatrixFrom(t *testing.T) {
	locations := []string{"Melbourne", "Toronto", "Prague", "Paris", "Tokyo", "Amsterdam", "Auckland", "Moscow", "Stockholm", "London"}
	xm := Matrix{}
	if xm.Enabled() {
		t.Errorf("Matrix{}.Enabled() = true, want false")
	}
	lm := MatrixFrom(locations)
	if !lm.Enabled() {
		t.Errorf("MatrixFrom(%v).Enabled() = false, want true", locations)
	}
	if len(lm.lm) != len(locations) {
		t.Errorf("len(MatrixFrom(%v)) = %d, want %d", locations, len(lm.lm), len(locations))
	}
	for i, fromLoc := range locations {
		id1 := hotstuff.ID(i + 1)
		for j, toLoc := range locations {
			id2 := hotstuff.ID(j + 1)
			// We can lookup the latency Between location names using the global allLatencies matrix
			// or by using the Latency method on the latency.Matrix created by MatrixFrom.
			locLatency := Between(fromLoc, toLoc)
			lmLatency := lm.Latency(id1, id2)
			if locLatency != lmLatency {
				t.Errorf("Latency(%s, %s) != lm.LatencyID(%d, %d) ==> %v != %v", fromLoc, toLoc, id1, id2, locLatency, lmLatency)
			}
		}
	}
}

func TestLatencyMatrixFromDefault(t *testing.T) {
	lm := MatrixFrom([]string{DefaultLocation})
	if lm.Enabled() {
		t.Errorf("Matrix{}.Enabled() = true, want false")
	}
	if len(lm.lm) != 0 {
		t.Errorf("len(Matrix(%v)) = %d, want 0", []string{DefaultLocation}, len(lm.lm))
	}
}

func TestLocation(t *testing.T) {
	locations := []string{"Melbourne", "Toronto", "Prague", "Paris", "Tokyo", "Amsterdam", "Auckland", "Moscow", "Stockholm", "London"}
	lm := MatrixFrom(locations)
	for i := range len(locations) {
		id := hotstuff.ID(i + 1)
		loc := lm.Location(id)
		if loc != locations[i] {
			t.Errorf("Location(%d) = %s, want %s", id, loc, locations[i])
		}
	}
	if lm.Location(0) != DefaultLocation {
		t.Errorf("Location(0) = %s, want %s", lm.Location(0), DefaultLocation)
	}

	// test that lm.Location panics if the ID is out of range
	outOfRangeID := hotstuff.ID(len(locations) + 1)
	defer func() {
		if r := recover(); r != nil {
			if r != "ID 11 out of range" {
				t.Errorf("Recovered from panic: %v, want ID 11 out of range", r)
			}
		} else {
			t.Errorf("Location(%d) did not panic", outOfRangeID)
		}
	}()
	if loc := lm.Location(outOfRangeID); loc != "" {
		t.Errorf("Location(%d) = %s, want empty string and panic", outOfRangeID, loc)
	}
}
