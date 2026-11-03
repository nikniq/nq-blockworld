package main

import (
	"math/rand"
	"testing"
)

// A relight of any region of an already-lit world must change nothing: the
// region is seeded from the light around it and from sky, torches and glow,
// so if the seeding ever misses a source the region comes out darker.
func TestRelightRegionIsIdempotent(t *testing.T) {
	w := NewWorld()
	w.relight = map[int]*editBox{} // generation leaves edits queued; start from a settled world
	w.relightRegion(0, 0, worldW, worldD, 0, worldH)
	before := append([]uint8(nil), w.Light...)
	r := rand.New(rand.NewSource(7))
	for i := 0; i < 40; i++ {
		x0, z0 := r.Intn(worldW), r.Intn(worldD)
		x1, z1 := x0+1+r.Intn(60), z0+1+r.Intn(60)
		y0 := r.Intn(worldH)
		y1 := min(worldH, y0+1+r.Intn(40))
		w.relightRegion(x0, z0, x1, z1, y0, y1)
		for k := range w.Light {
			if w.Light[k] != before[k] {
				y := k / (worldW * worldD)
				z := (k / worldW) % worldD
				x := k % worldW
				t.Fatalf("region %d (%d,%d)-(%d,%d) y %d-%d changed light at %d,%d,%d: %#x -> %#x", i, x0, z0, x1, z1, y0, y1, x, y, z, before[k], w.Light[k])
			}
		}
	}
}

// Placing a block under open sky must darken the column beneath it and
// nothing else once the edit is flushed, matching a from-scratch relight.
func TestEditRelightMatchesFullRelight(t *testing.T) {
	w := NewWorld()
	w.relight = map[int]*editBox{}
	w.relightRegion(0, 0, worldW, worldD, 0, worldH)
	x, z := worldW/2+originX, worldD/2+originZ
	y := w.SurfaceY(x, z) + 6
	w.Set(x, y, z, Stone)
	w.Set(x+1, y, z, Leaves)
	w.Set(x+3, y-2, z+2, Torch)
	w.flushLight()
	for len(w.relight) > 0 {
		w.flushLight()
	}
	got := append([]uint8(nil), w.Light...)
	w.relightRegion(0, 0, worldW, worldD, 0, worldH)
	for k := range w.Light {
		if w.Light[k] != got[k] {
			t.Fatalf("incremental relight differs from full relight at cell %d: %#x vs %#x", k, got[k], w.Light[k])
		}
	}
}
