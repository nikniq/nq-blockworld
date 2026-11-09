package main

import rl "github.com/gen2brain/raylib-go/raylib"

// Homewares: bunk beds, tables, chairs and vases. Each is a single block
// drawn as several small boxes. Nothing about their facing is stored: a
// chair turns its back away from a table beside it (or against a wall), a
// row of tables shares legs, and a bunk bed hangs its ladder on an open side.

const (
	tableTop = 0.9375 // height of a table's surface within its cell
	chairTop = 0.5625 // height of a chair's seat
	bunkTop  = 0.45   // height of a bunk mattress above the floor of its cell
	rugTop   = 0.0625 // thickness of a rug
)

// isBed reports whether a block is somewhere to sleep and set a spawn point.
func isBed(b Block) bool { return b == Bed || b == BunkBed || b == BunkTop }

// flowerKinds are the wild flowers, for world generation and vases.
var flowerKinds = []Block{Flower, FlowerYellow, FlowerBlue, FlowerWhite, FlowerPink}

// furnitureBoxes returns the boxes that draw a piece of furniture and the
// block whose texture each one wears, or nil for any other block.
func (w *World) furnitureBoxes(lx, y, lz int, b Block) (boxes [][2][3]float32, tex []Block) {
	add := func(t Block, x0, y0, z0, x1, y1, z1 float32) {
		boxes = append(boxes, [2][3]float32{{x0, y0, z0}, {x1, y1, z1}})
		tex = append(tex, t)
	}
	const leg = 0.125
	switch b {
	case Table:
		add(Planks, 0, 0.8125, 0, 1, tableTop, 1)
		// A leg at every corner that does not run on into another table.
		for _, c := range [4][2]int{{-1, -1}, {1, -1}, {-1, 1}, {1, 1}} {
			if w.getLocal(lx+c[0], y, lz) == Table || w.getLocal(lx, y, lz+c[1]) == Table {
				continue
			}
			x0, z0 := float32(0.0625), float32(0.0625)
			if c[0] > 0 {
				x0 = 1 - 0.0625 - leg
			}
			if c[1] > 0 {
				z0 = 1 - 0.0625 - leg
			}
			add(Log, x0, 0, z0, x0+leg, 0.8125, z0+leg)
		}
	case Chair:
		add(Planks, 0.125, 0.4375, 0.125, 0.875, chairTop, 0.875)
		for _, c := range [4][2]float32{{0.125, 0.125}, {0.75, 0.125}, {0.125, 0.75}, {0.75, 0.75}} {
			add(Log, c[0], 0, c[1], c[0]+leg, 0.4375, c[1]+leg)
		}
		// The back: two uprights and two slats on the side the sitter leans against.
		dx, dz := w.chairBack(lx, y, lz)
		switch {
		case dz < 0:
			add(Log, 0.125, chairTop, 0.125, 0.25, 1.25, 0.25)
			add(Log, 0.75, chairTop, 0.125, 0.875, 1.25, 0.25)
			add(Planks, 0.25, 0.8, 0.15, 0.75, 0.95, 0.225)
			add(Planks, 0.25, 1.05, 0.15, 0.75, 1.2, 0.225)
		case dz > 0:
			add(Log, 0.125, chairTop, 0.75, 0.25, 1.25, 0.875)
			add(Log, 0.75, chairTop, 0.75, 0.875, 1.25, 0.875)
			add(Planks, 0.25, 0.8, 0.775, 0.75, 0.95, 0.85)
			add(Planks, 0.25, 1.05, 0.775, 0.75, 1.2, 0.85)
		case dx < 0:
			add(Log, 0.125, chairTop, 0.125, 0.25, 1.25, 0.25)
			add(Log, 0.125, chairTop, 0.75, 0.25, 1.25, 0.875)
			add(Planks, 0.15, 0.8, 0.25, 0.225, 0.95, 0.75)
			add(Planks, 0.15, 1.05, 0.25, 0.225, 1.2, 0.75)
		default:
			add(Log, 0.75, chairTop, 0.125, 0.875, 1.25, 0.25)
			add(Log, 0.75, chairTop, 0.75, 0.875, 1.25, 0.875)
			add(Planks, 0.775, 0.8, 0.25, 0.85, 0.95, 0.75)
			add(Planks, 0.775, 1.05, 0.25, 0.85, 1.2, 0.75)
		}
	case BunkBed:
		for _, c := range [4][2]float32{{0, 0}, {1 - leg, 0}, {0, 1 - leg}, {1 - leg, 1 - leg}} {
			add(Log, c[0], 0, c[1], c[0]+leg, 1.8, c[1]+leg)
		}
		for _, base := range []float32{0.1875, 1.1875} {
			add(Planks, 0, base, 0, 1, base+0.125, 1)                         // frame
			add(Bed, 0.0625, base+0.125, 0.0625, 0.9375, base+0.2625, 0.9375) // mattress and blanket
			add(Wool, 0.2, base+0.2625, 0.1, 0.8, base+0.33, 0.3)             // pillow
		}
		// Ladder rungs up an open side, and a guard rail above it.
		dx, dz := 1, 0
		for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			if !blocks[w.getLocal(lx+d[0], y, lz+d[1])].Solid {
				dx, dz = d[0], d[1]
				break
			}
		}
		for _, ry := range []float32{0.55, 0.8, 1.05} {
			switch {
			case dx > 0:
				add(Planks, 0.94, ry, 0.3, 1, ry+0.07, 0.7)
			case dx < 0:
				add(Planks, 0, ry, 0.3, 0.06, ry+0.07, 0.7)
			case dz > 0:
				add(Planks, 0.3, ry, 0.94, 0.7, ry+0.07, 1)
			default:
				add(Planks, 0.3, ry, 0, 0.7, ry+0.07, 0.06)
			}
		}
		switch {
		case dx != 0:
			x0 := float32(0)
			if dx < 0 {
				x0 = 0.94
			}
			add(Planks, x0, 1.55, 0.125, x0+0.06, 1.65, 0.875) // rail on the far side from the ladder
		default:
			z0 := float32(0)
			if dz < 0 {
				z0 = 0.94
			}
			add(Planks, 0.125, 1.55, z0, 0.875, 1.65, z0+0.06)
		}
	case Lamp:
		add(Log, 0.375, 0, 0.375, 0.625, 0.06, 0.625)  // foot
		add(Log, 0.46, 0.06, 0.46, 0.54, 0.7, 0.54)    // stem
		add(Wool, 0.25, 0.62, 0.25, 0.75, 1.02, 0.75)  // shade, lit from inside
		add(Torch, 0.44, 1.02, 0.44, 0.56, 1.06, 0.56) // the glow showing at the top
	case Sofa:
		// Laid out with the back along one side; a run of sofas shares one
		// long seat, with an arm only at each end.
		dx, dz := w.sofaBack(lx, y, lz)
		alongX := dz != 0 // the seat runs along X when the back is on a Z side
		far := dx > 0 || dz > 0
		part := func(t Block, u0, u1, y0, y1, v0, v1 float32) {
			if far {
				v0, v1 = 1-v1, 1-v0
			}
			if alongX {
				add(t, u0, y0, v0, u1, y1, v1)
			} else {
				add(t, v0, y0, u0, v1, y1, u1)
			}
		}
		part(Planks, 0, 1, 0.06, 0.2, 0, 1)        // frame
		part(RugRed, 0, 1, 0.2, chairTop, 0.15, 1) // seat cushion
		part(RugRed, 0, 1, 0.2, 1.05, 0, 0.2)      // back
		lo, hi := w.getLocal(lx-1, y, lz), w.getLocal(lx+1, y, lz)
		if !alongX {
			lo, hi = w.getLocal(lx, y, lz-1), w.getLocal(lx, y, lz+1)
		}
		if lo != Sofa {
			part(RugRed, 0, 0.14, 0.2, 0.8, 0, 1)
			part(Log, 0.03, 0.13, 0, 0.06, 0.05, 0.15)
			part(Log, 0.03, 0.13, 0, 0.06, 0.85, 0.95)
		}
		if hi != Sofa {
			part(RugRed, 0.86, 1, 0.2, 0.8, 0, 1)
			part(Log, 0.87, 0.97, 0, 0.06, 0.05, 0.15)
			part(Log, 0.87, 0.97, 0, 0.06, 0.85, 0.95)
		}
	case Vase:
		add(RedSand, 0.3125, 0, 0.3125, 0.6875, 0.3, 0.6875)
		add(RedSand, 0.25, 0.3, 0.25, 0.75, 0.375, 0.75) // rim
	}
	return boxes, tex
}

// chairBack returns the direction of the side a chair's back stands on:
// away from a table if one is beside it, otherwise against a wall, otherwise -Z.
func (w *World) chairBack(lx, y, lz int) (dx, dz int) {
	dirs := [4][2]int{{0, -1}, {0, 1}, {-1, 0}, {1, 0}}
	for _, d := range dirs {
		if w.getLocal(lx+d[0], y, lz+d[1]) == Table {
			return -d[0], -d[1]
		}
	}
	for _, d := range dirs {
		if nb := w.getLocal(lx+d[0], y, lz+d[1]); nb.Opaque() && blocks[nb].Solid {
			return d[0], d[1]
		}
	}
	return 0, -1
}

// sofaBack returns the side a sofa's back stands on: against a wall if there
// is one, otherwise in line with a sofa beside it, otherwise -Z.
func (w *World) sofaBack(lx, y, lz int) (dx, dz int) {
	dirs := [4][2]int{{0, -1}, {0, 1}, {-1, 0}, {1, 0}}
	for _, d := range dirs {
		if nb := w.getLocal(lx+d[0], y, lz+d[1]); nb.Opaque() && blocks[nb].Solid {
			return d[0], d[1]
		}
	}
	if w.getLocal(lx, y, lz-1) == Sofa || w.getLocal(lx, y, lz+1) == Sofa {
		return -1, 0 // a row running along Z
	}
	return 0, -1
}

// wallPanel is the thin square of a painting or a clock, hung flat on
// whichever wall the cell touches, or standing mid-cell with none.
func (w *World) wallPanel(lx, y, lz int) [2][3]float32 {
	const t = 0.06
	switch {
	case w.getLocal(lx, y, lz-1).Opaque():
		return [2][3]float32{{0, 0, 0.01}, {1, 1, 0.01 + t}}
	case w.getLocal(lx, y, lz+1).Opaque():
		return [2][3]float32{{0, 0, 0.99 - t}, {1, 1, 0.99}}
	case w.getLocal(lx-1, y, lz).Opaque():
		return [2][3]float32{{0.01, 0, 0}, {0.01 + t, 1, 1}}
	case w.getLocal(lx+1, y, lz).Opaque():
		return [2][3]float32{{0.99 - t, 0, 0}, {0.99, 1, 1}}
	}
	return panelBox
}

// SetBunk writes both halves of a bunk bed whose lower bunk is at y.
func (w *World) SetBunk(x, y, z int) {
	w.Set(x, y, z, BunkBed)
	w.Set(x, y+1, z, BunkTop)
}

// sit puts the player on a chair. Moving or jumping stands them up again.
func (g *Game) sit(x, y, z int) {
	p := g.Player
	p.Pos = rl.NewVector3(float32(x)+0.5, float32(y)+chairTop, float32(z)+0.5) // chairs and sofas share a seat height
	p.VelY = 0
	p.Sitting = true
	g.say("Sitting. Move to stand up", 1.5)
}
