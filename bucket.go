package main

import rl "github.com/gen2brain/raylib-go/raylib"

// Buckets carry water. An empty bucket scoops the water cell it is aimed at
// (seas and lakes are bottomless: a cell with two or more watery neighbours
// refills itself); a full one pours into the air cell in front of the aimed
// face, and the water falls until it meets ground. Water poured onto lava
// quenches it to cobblestone.

const pourDepth = 12 // how far poured water falls

// useBucket runs the held bucket for one frame.
func (g *Game) useBucket() {
	p, w := g.Player, g.World
	p.Aim = w.RayCastLiquid(p.Eye(), p.Forward(), reachDist)
	if !usePressed() || !p.Aim.Hit || !w.InBounds(p.Aim.X, p.Aim.Y, p.Aim.Z) {
		return
	}
	a := p.Aim
	if p.Held.Block == Bucket {
		if w.Get(a.X, a.Y, a.Z) != Water {
			g.say("Aim at water to fill the bucket", 1.2)
			return
		}
		g.bucketAt(a.X, a.Y, a.Z, Air)
		p.Inv[Bucket]--
		p.Inv[WaterBucket]++
		g.Audio.Play(g.Audio.Splash, 0.5)
		p.Swing = 1
		p.EnsureHeld()
		return
	}
	x, y, z := a.X+a.NX, a.Y+a.NY, a.Z+a.NZ
	if t := w.Get(a.X, a.Y, a.Z); t == Water || t == Lava {
		x, y, z = a.X, a.Y, a.Z // pouring onto a liquid tops it up or quenches it
	}
	t := w.Get(x, y, z)
	if !w.InBounds(x, y, z) || (t != Air && t != Lava) {
		g.say("Nowhere to pour here", 1.2)
		return
	}
	g.bucketAt(x, y, z, Water)
	if !settings.Creative {
		p.Inv[WaterBucket]--
		p.Inv[Bucket]++
	}
	g.Audio.Play(g.Audio.Splash, 0.6)
	p.Swing = 1
	p.EnsureHeld()
}

// bucketAt applies a scoop (b == Air) or a pour (b == Water) locally, or asks the host to.
func (g *Game) bucketAt(x, y, z int, b Block) {
	if g.isClient() {
		g.sendToHost(&Msg{Place: &struct {
			X, Y, Z int
			B       Block
		}{x, y, z, b}})
		return
	}
	g.applyBucket(x, y, z, b)
}

// applyBucket changes the world for a bucket action (host and solo).
func (g *Game) applyBucket(x, y, z int, b Block) {
	w := g.World
	switch b {
	case Air:
		if w.Get(x, y, z) != Water {
			return
		}
		n := 0
		for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			if w.Get(x+d[0], y, z+d[1]) == Water {
				n++
			}
		}
		if n >= 2 {
			return // a body of water: it refills at once
		}
		w.Set(x, y, z, Air)
	case Water:
		if w.Get(x, y, z) == Lava {
			w.Set(x, y, z, Cobble)
			g.burst(rl.NewVector3(float32(x)+0.5, float32(y)+1, float32(z)+0.5), rl.NewColor(220, 220, 230, 255), 16)
			return
		}
		if w.Get(x, y, z) != Air {
			return
		}
		for i := 0; i < pourDepth; i++ {
			w.Set(x, y, z, Water)
			if y == 0 || w.Get(x, y-1, z) != Air {
				break
			}
			y--
		}
		if w.Get(x, y-1, z) == Lava {
			w.Set(x, y-1, z, Cobble)
		}
	}
	g.sendFx(Fx{Kind: FxPlace, Pos: rl.NewVector3(float32(x), float32(y), float32(z))})
}
