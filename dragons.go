package main

import (
	"fmt"
	"math"
	"math/rand"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Four dragons: a fire dragon roosting on the highest peak, a frost dragon
// over the snowy taiga, an earth dragon on the desert mesa and a water dragon
// out at sea. They circle their roosts, dive and breathe at anyone who comes
// near, and drop a hoard when slain. Fire and frost breathe fireballs (frost
// chills), earth hurls boulders and slams the ground so it heaves, water
// blasts jets that knock you flying.

type DragonState int

const (
	DragonCircle DragonState = iota
	DragonAttack
	DragonDying
	DragonDead
)

type DragonKind uint8

const (
	DragonFire DragonKind = iota
	DragonFrost
	DragonEarth
	DragonWater
	numDragonKinds
)

var dragonNames = [numDragonKinds]string{"Fire Dragon", "Frost Dragon", "Earth Dragon", "Water Dragon"}

type Dragon struct {
	ID     int
	Name   string
	Frost  bool // older saves; see Kind
	Kind   DragonKind
	Pos    rl.Vector3
	Vel    rl.Vector3
	Roost  rl.Vector3
	HP     int
	MaxHP  int
	State  DragonState
	Angle  float32 // circling angle
	Flap   float32
	FireCD float32
	BiteCD float32
	RoarCD float32
	Anger  float32 // seconds it stays hostile after being hurt
	Flash  float32
	DeathT float32
	Target uint32 // player id it is attacking
}

// Fireball is any dragon's breath: a fireball, a frost ball, a boulder or a water jet.
type Fireball struct {
	Pos   rl.Vector3
	Vel   rl.Vector3
	Life  float32
	Frost bool // older saves; see Kind
	Kind  DragonKind
}

// hp, circling height and speed per kind.
var dragonStats = [numDragonKinds]struct {
	HP     int
	Height float32
	Speed  float32
}{
	{150, 14, 10}, {150, 14, 10}, {220, 9, 8}, {180, 7, 11},
}

const (
	dragonHP     = 150
	dragonAlert  = 40.0 // blocks from the roost that wake it
	dragonChase  = 70.0 // beyond this it gives up
	dragonHeight = 14.0 // circling altitude of the fire dragon, used by the debug line-up
)

// spawnDragons picks four roosts far apart: the highest point, the highest
// snowy point, the highest sandy point and a spot out at sea.
func (g *Game) spawnDragons() {
	w := g.World
	g.Dragons = nil
	best, snowBest, sandBest, seaBest := -1, -1, -1, -1
	var bx, bz, sx, sz, ex, ez, wx, wz int
	farFromSpawn := func(lx, lz int) bool {
		dx, dz := lx+originX, lz+originZ
		return dx*dx+dz*dz >= 60*60
	}
	// apart reports whether a cell is at least 48 blocks from every roost chosen so far.
	apart := func(lx, lz int, roosts ...[2]int) bool {
		for _, r := range roosts {
			dx, dz := abs(lx-r[0]), abs(lz-r[1])
			dx, dz = min(dx, worldW-dx), min(dz, worldD-dz)
			if dx*dx+dz*dz < 48*48 {
				return false
			}
		}
		return true
	}
	// highest finds the tallest dry column whose top block passes ok, away from the given roosts.
	highest := func(ok func(Block) bool, roosts ...[2]int) (h, lx, lz int) {
		h = -1
		for try := 0; try < 4000; try++ {
			cx, cz := rand.Intn(worldW), rand.Intn(worldD)
			ch := w.Ground[cz*worldW+cx]
			if ch <= seaLevel+2 || ch <= h || !farFromSpawn(cx, cz) || !apart(cx, cz, roosts...) || !ok(w.getLocal(cx, ch-1, cz)) {
				continue
			}
			h, lx, lz = ch, cx, cz
		}
		return
	}
	best, bx, bz = highest(func(Block) bool { return true })
	if best < 0 {
		return
	}
	snowBest, sx, sz = highest(func(b Block) bool { return b == Snow }, [2]int{bx, bz})
	sandBest, ex, ez = highest(func(b Block) bool { return b == Sand || b == RedSand }, [2]int{bx, bz}, [2]int{sx, sz})
	if snowBest < 0 || (sx == bx && sz == bz) {
		// No snow: put the frost dragon on the far side of the world instead.
		sx, sz = wrapX(bx+worldW/2), wrapZ(bz+worldD/2)
		snowBest = w.Ground[sz*worldW+sx]
	}
	if sandBest < 0 || (ex == bx && ez == bz) {
		ex, ez = wrapX(bx+worldW/2), wrapZ(bz)
		sandBest = w.Ground[ez*worldW+ex]
	}
	// The water dragon wants open sea: a deep cell with water all around, as
	// far from the other roosts as can be found.
	bestScore := -1
	for try := 0; try < 4000; try++ {
		lx, lz := rand.Intn(worldW), rand.Intn(worldD)
		if !farFromSpawn(lx, lz) || w.Ground[lz*worldW+lx] > seaLevel-3 {
			continue
		}
		open := true
		for _, d := range [4][2]int{{8, 0}, {-8, 0}, {0, 8}, {0, -8}} {
			if w.Ground[wrapZ(lz+d[1])*worldW+wrapX(lx+d[0])] > seaLevel-1 {
				open = false
			}
		}
		if !open {
			continue
		}
		score := 1 << 30
		for _, r := range [3][2]int{{bx, bz}, {sx, sz}, {ex, ez}} {
			dx, dz := abs(lx-r[0]), abs(lz-r[1])
			dx, dz = min(dx, worldW-dx), min(dz, worldD-dz)
			score = min(score, dx*dx+dz*dz)
		}
		if score > bestScore {
			bestScore, wx, wz = score, lx, lz
		}
	}
	if bestScore < 0 {
		wx, wz = wrapX(bx), wrapZ(bz+worldD/2)
		seaBest = w.Ground[wz*worldW+wx]
	} else {
		seaBest = seaLevel
	}
	mk := func(id int, kind DragonKind, lx, h, lz int) {
		st := dragonStats[kind]
		roost := rl.NewVector3(float32(lx+originX)+0.5, float32(h), float32(lz+originZ)+0.5)
		d := &Dragon{ID: id, Name: dragonNames[kind], Kind: kind, Frost: kind == DragonFrost, Roost: roost, HP: st.HP, MaxHP: st.HP, Angle: rand.Float32() * 6.28}
		d.Pos = rl.NewVector3(roost.X+22, roost.Y+st.Height, roost.Z)
		g.Dragons = append(g.Dragons, d)
	}
	mk(1, DragonFire, bx, best, bz)
	mk(2, DragonFrost, sx, snowBest, sz)
	mk(3, DragonEarth, ex, sandBest, ez)
	mk(4, DragonWater, wx, seaBest, wz)
}

// upgradeDragons maps dragons from older saves, which only knew fire and frost, onto kinds.
func (g *Game) upgradeDragons() {
	for _, d := range g.Dragons {
		if d.Kind == DragonFire && d.Frost {
			d.Kind = DragonFrost
		}
		if d.Name == "" {
			d.Name = dragonNames[d.Kind]
		}
	}
	for i := range g.Fireballs {
		if f := &g.Fireballs[i]; f.Kind == DragonFire && f.Frost {
			f.Kind = DragonFrost
		}
	}
}

// tickDragons runs the dragons and their fireballs (host and solo).
func (g *Game) tickDragons(dt float32) {
	for _, d := range g.Dragons {
		g.tickDragon(d, dt)
	}
	// Breath projectiles.
	keep := g.Fireballs[:0]
	for i := range g.Fireballs {
		f := &g.Fireballs[i]
		f.Life -= dt
		switch f.Kind {
		case DragonEarth:
			f.Vel.Y -= 9 * dt // boulders are heavy
		case DragonWater:
			f.Vel.Y -= 1 * dt
		default:
			f.Vel.Y -= 3 * dt
		}
		next := rl.Vector3Add(f.Pos, rl.Vector3Scale(f.Vel, dt))
		hit := f.Life <= 0 || g.World.Solid(floorI(next.X), floorI(next.Y), floorI(next.Z))
		for _, t := range g.targets() {
			if WrapDist(next, rl.Vector3Add(t.Pos, rl.NewVector3(0, 0.9, 0))) < 1.3 {
				hit = true
			}
		}
		if hit {
			g.breathHits(f, next)
			continue
		}
		f.Pos = WrapPos(next)
		g.Sparks = append(g.Sparks, Spark{Pos: f.Pos, Vel: rl.NewVector3(rand.Float32()-0.5, rand.Float32(), rand.Float32()-0.5), Life: 0.25, Col: fireColor(f.Kind)})
		keep = append(keep, *f)
	}
	g.Fireballs = keep
}

// breathHits lands a projectile.
func (g *Game) breathHits(f *Fireball, at rl.Vector3) {
	switch f.Kind {
	case DragonWater:
		// A jet of water: no scorched ground, but it knocks hard.
		g.burst(at, rl.NewColor(120, 180, 255, 255), 24)
		g.Audio.Play(g.Audio.Splash, 0.8)
		for _, t := range g.targets() {
			if d := WrapDist(at, rl.Vector3Add(t.Pos, rl.NewVector3(0, 0.9, 0))); d < 4 {
				g.hurtTargetFrom(t.ID, int(14*(1-d/5)*damageScale()), "was swept away by the Water Dragon", true, at, 16*(1-d/5))
			}
		}
	case DragonEarth:
		g.blast(at, 1.4, 30)
		g.Shake = max(g.Shake, 0.5)
		g.burst(at, rl.NewColor(150, 120, 90, 255), 20)
		if rand.Float32() < 0.5 {
			g.spawnDrop(at, Cobble, 0)
		}
	default:
		g.blast(at, 1.6, 34)
		if f.Kind == DragonFrost {
			g.burst(at, rl.NewColor(180, 230, 255, 255), 20)
			for _, t := range g.targets() {
				if t.ID == 0 && WrapDist(at, t.Pos) < 4 {
					g.Player.Knock = rl.Vector3Scale(g.Player.Knock, 0.2)
					g.Player.FrostT = 3 // slowed
				}
			}
		}
	}
}

func fireColor(kind DragonKind) rl.Color {
	switch kind {
	case DragonFrost:
		return rl.NewColor(150, 220, 255, 255)
	case DragonEarth:
		return rl.NewColor(140, 110, 80, 255)
	case DragonWater:
		return rl.NewColor(80, 150, 255, 255)
	}
	return rl.NewColor(255, 140, 40, 255)
}

func (g *Game) tickDragon(d *Dragon, dt float32) {
	d.Flash = max(0, d.Flash-dt*3)
	d.FireCD = max(0, d.FireCD-dt)
	d.BiteCD = max(0, d.BiteCD-dt)
	d.RoarCD = max(0, d.RoarCD-dt)
	d.Anger = max(0, d.Anger-dt)
	d.Flap += dt * 6
	switch d.State {
	case DragonDead:
		return
	case DragonDying:
		d.DeathT += dt
		d.Vel.Y -= 12 * dt
		d.Pos = rl.Vector3Add(d.Pos, rl.Vector3Scale(d.Vel, dt))
		if g.World.Solid(floorI(d.Pos.X), floorI(d.Pos.Y), floorI(d.Pos.Z)) || d.DeathT > 6 {
			g.dragonFalls(d)
		}
		return
	}
	// Who is near? Players only (dragons ignore the villages' wars).
	var target *Target
	var td float32 = 1e9
	for _, t := range g.targets() {
		if t.ID&villagerIDBit != 0 {
			continue
		}
		if dist := WrapDist(t.Pos, d.Roost); dist < td {
			tt := t
			target, td = &tt, dist
		}
	}
	if d.State == DragonCircle {
		if target != nil && (td < dragonAlert || d.Anger > 0 && WrapDist(target.Pos, d.Pos) < dragonChase) {
			d.State = DragonAttack
			d.Target = target.ID
			g.announce("The "+d.Name+" has seen you!", 3)
			g.Audio.Play(g.Audio.Roar, 1)
		}
	}
	switch d.State {
	case DragonCircle:
		st := dragonStats[d.Kind]
		d.Angle += dt * st.Speed / 24
		want := rl.NewVector3(d.Roost.X+float32(math.Cos(float64(d.Angle)))*24, d.Roost.Y+st.Height+float32(math.Sin(float64(d.Angle*2)))*2, d.Roost.Z+float32(math.Sin(float64(d.Angle)))*24)
		g.dragonSteer(d, want, st.Speed, dt)
		if d.RoarCD == 0 {
			d.RoarCD = 20 + rand.Float32()*20
			if WrapDist(d.Pos, g.Player.Pos) < 90 {
				g.Audio.Play(g.Audio.Roar, 0.6*clamp(1-WrapDist(d.Pos, g.Player.Pos)/90, 0, 1))
			}
		}
	case DragonAttack:
		if target == nil || (WrapDist(target.Pos, d.Roost) > dragonChase && d.Anger == 0) {
			d.State = DragonCircle
			return
		}
		st := dragonStats[d.Kind]
		tp := Near(target.Pos, d.Pos)
		flat := rl.Vector3Subtract(tp, d.Pos)
		flat.Y = 0
		hd := rl.Vector3Length(flat)
		// Swoop: dive to bite when the breath is on cooldown, else hover above and breathe.
		var want rl.Vector3
		hover := float32(9)
		if d.Kind == DragonEarth {
			hover = 6 // stays low, ready to slam
		}
		if d.FireCD > 1.2 && hd < 12 {
			want = rl.NewVector3(tp.X, tp.Y+1.5, tp.Z)
		} else {
			want = rl.NewVector3(tp.X-flat.X/max(hd, 1)*8, tp.Y+hover, tp.Z-flat.Z/max(hd, 1)*8)
		}
		g.dragonSteer(d, want, st.Speed*1.2, dt)
		if hd < 16 && d.FireCD == 0 && math.Abs(float64(d.Pos.Y-tp.Y)) < 14 {
			from := rl.Vector3Add(d.Pos, rl.Vector3Scale(rl.Vector3Normalize(d.Vel), 2.2))
			to := rl.Vector3Add(tp, rl.NewVector3(0, 1, 0))
			dir := rl.Vector3Normalize(rl.Vector3Subtract(to, from))
			switch d.Kind {
			case DragonEarth:
				d.FireCD = 4
				// Lob a boulder: aim a little high so gravity brings it down on the target.
				dir.Y += hd / 40
				g.Fireballs = append(g.Fireballs, Fireball{Pos: from, Vel: rl.Vector3Scale(rl.Vector3Normalize(dir), 15), Life: 4, Kind: d.Kind})
				g.Audio.Play(g.Audio.Dig, 1)
			case DragonWater:
				d.FireCD = 2.5
				g.Fireballs = append(g.Fireballs, Fireball{Pos: from, Vel: rl.Vector3Scale(dir, 24), Life: 2, Kind: d.Kind})
				g.Audio.Play(g.Audio.Splash, 0.9)
			default:
				d.FireCD = 3.5
				g.Fireballs = append(g.Fireballs, Fireball{Pos: from, Vel: rl.Vector3Scale(dir, 18), Life: 3, Kind: d.Kind, Frost: d.Kind == DragonFrost})
				g.Audio.Play(g.Audio.Burn, 0.9)
			}
		}
		if rl.Vector3Distance(d.Pos, rl.Vector3Add(tp, rl.NewVector3(0, 1, 0))) < 3 && d.BiteCD == 0 {
			d.BiteCD = 2
			if d.Kind == DragonEarth {
				d.BiteCD = 5
				g.earthSlam(d, tp)
			} else {
				g.hurtTargetFrom(target.ID, int(22*damageScale()), "was devoured by the "+d.Name, true, d.Pos, 9)
			}
		}
	}
}

// earthSlam is the earth dragon landing on its prey: the ground heaves in a
// ring around the strike, anything near is hurt and thrown, and the screen shakes.
func (g *Game) earthSlam(d *Dragon, at rl.Vector3) {
	w := g.World
	g.Shake = max(g.Shake, 1.2)
	g.Audio.Play(g.Audio.Thunder, 0.8)
	g.burst(at, rl.NewColor(150, 120, 90, 255), 40)
	for _, t := range g.targets() {
		if dist := WrapDist(at, t.Pos); dist < 7 {
			g.hurtTargetFrom(t.ID, int(20*(1-dist/9)*damageScale()), "was crushed by the Earth Dragon", true, at, 12*(1-dist/9))
		}
	}
	cx, cz := floorI(at.X), floorI(at.Z)
	for oz := -4; oz <= 4; oz++ {
		for ox := -4; ox <= 4; ox++ {
			r := ox*ox + oz*oz
			if r < 2 || r > 16 || rand.Float32() < 0.45 {
				continue
			}
			x, z := cx+ox, cz+oz
			ground := w.SurfaceY(x, z)
			if ground <= seaLevel+1 || ground < quakeDepth+3 || g.quakeProtected(x, z) {
				continue
			}
			g.shiftColumn(x, z, ground, 1)
		}
	}
	g.unburyAll(at, 6)
	g.sendFx(Fx{Kind: FxQuake, Pos: at, Shake: 1.2})
}

// dragonSteer eases the dragon toward a point, keeping it above the ground.
func (g *Game) dragonSteer(d *Dragon, want rl.Vector3, speed float32, dt float32) {
	to := rl.Vector3Subtract(Near(want, d.Pos), d.Pos)
	if l := rl.Vector3Length(to); l > 0.01 {
		to = rl.Vector3Scale(to, speed/l)
	}
	d.Vel = rl.Vector3Add(d.Vel, rl.Vector3Scale(rl.Vector3Subtract(to, d.Vel), min(dt*1.5, 1)))
	d.Pos = rl.Vector3Add(d.Pos, rl.Vector3Scale(d.Vel, dt))
	ground := float32(g.World.SurfaceY(floorI(d.Pos.X), floorI(d.Pos.Z)))
	if d.Pos.Y < ground+2 {
		d.Pos.Y = ground + 2
		d.Vel.Y = max(d.Vel.Y, 0)
	}
	d.Pos = WrapPos(d.Pos)
}

// BB is the dragon's hit box.
func (d *Dragon) BB() rl.BoundingBox {
	return rl.NewBoundingBox(rl.Vector3Subtract(d.Pos, rl.NewVector3(2.2, 1.2, 2.2)), rl.Vector3Add(d.Pos, rl.NewVector3(2.2, 1.4, 2.2)))
}

// hitDragon finds the first living dragon along a ray within maxD (boxes brought near the origin).
func (g *Game) hitDragon(ray rl.Ray, maxD float32) (*Dragon, float32) {
	var best *Dragon
	bd := maxD
	for _, d := range g.Dragons {
		if d.State == DragonDead || d.State == DragonDying {
			continue
		}
		bb := d.BB()
		n := Near(d.Pos, ray.Position)
		off := rl.Vector3Subtract(n, d.Pos)
		bb = rl.NewBoundingBox(rl.Vector3Add(bb.Min, off), rl.Vector3Add(bb.Max, off))
		if c := rl.GetRayCollisionBox(ray, bb); c.Hit && c.Distance < bd {
			best, bd = d, c.Distance
		}
	}
	return best, bd
}

// damageDragon applies a hit; dragons remember who hurt them.
func (g *Game) damageDragon(d *Dragon, dmg int) {
	if d.State == DragonDead || d.State == DragonDying {
		return
	}
	d.HP -= dmg
	d.Flash = 1
	d.Anger = 40
	if d.State == DragonCircle {
		d.State = DragonAttack
		g.announce("The "+d.Name+" turns on you!", 3)
		g.Audio.Play(g.Audio.Roar, 1)
	}
	if d.HP <= 0 {
		d.State = DragonDying
		d.DeathT = 0
		d.Vel = rl.NewVector3(d.Vel.X*0.3, -2, d.Vel.Z*0.3)
		g.announce("The "+d.Name+" is falling!", 3)
		g.Audio.Play(g.Audio.Roar, 1)
	}
}

func (g *Game) dragonFalls(d *Dragon) {
	d.State = DragonDead
	at := rl.NewVector3(d.Pos.X, d.Pos.Y+1, d.Pos.Z)
	g.blast(at, 2.2, 10)
	hoard := map[DragonKind][]Block{
		DragonFire:  {DiamondOre, DiamondOre, DiamondOre, DiamondOre, DiamondOre, GoldOre, GoldOre, GoldOre},
		DragonFrost: {DiamondOre, DiamondOre, DiamondOre, DiamondOre, DiamondOre, GoldOre, GoldOre, GoldOre},
		DragonEarth: {Amethyst, Amethyst, Amethyst, Amethyst, GoldOre, GoldOre, GoldOre, IronOre, IronOre, IronOre, IronOre, DiamondOre},
		DragonWater: {DiamondOre, DiamondOre, DiamondOre, DiamondOre, Fish, Fish, Fish, Fish, Fish, Fish, Amethyst, Amethyst},
	}
	for _, b := range hoard[d.Kind] {
		g.spawnDrop(at, b, 0)
	}
	g.spawnDrop(at, 0, 48)
	g.Score += 2500
	g.announce(fmt.Sprintf("%s SLAIN  +2500  -  its hoard lies where it fell", d.Name), 5)
	g.unlock(AchDragon)
	alive := 0
	for _, o := range g.Dragons {
		if o.State != DragonDead {
			alive++
		}
	}
	if alive == 0 {
		g.Score += 2500
		g.announce("All the dragons are dead. The skies are yours.  +2500", 6)
	}
}

// ---------- drawing ----------

// DrawDragon draws a dragon: long body, neck and head, tail, two flapping wings, tucked legs.
func (s *Skins) DrawDragon(w *World, p *Pose, kind DragonKind, flap float32) {
	s.init(w)
	model := modelMatrix(p)
	c := lightColor(w, p.Pos, p.Alpha)
	k := [numDragonKinds]SkinKind{SkinDragon, SkinFrostDragon, SkinEarthDragon, SkinWaterDragon}[kind]
	wing := float32(math.Sin(float64(flap))) * 0.6
	s.drawPart(k, PartBody, rl.NewVector3(1.2, 1.0, 3.2), rl.NewVector3(0, 0, 0), 0, 0, 0, 0, model, c)
	s.drawPart(k, PartArmL, rl.NewVector3(0.8, 0.7, 1.6), rl.NewVector3(0, 0.3, 2.2), 0, -0.4+p.Pitch, 0, 0, model, c) // neck
	s.drawPart(k, PartHead, rl.NewVector3(0.9, 0.7, 1.5), rl.NewVector3(0, 0.9, 3.4), 0, -0.1+p.Pitch+p.Swing*0.5, 0, 0, model, c)
	s.drawPart(k, PartExtra, rl.NewVector3(0.7, 0.6, 2.2), rl.NewVector3(0, 0.1, -2.6), 0, 0.15, float32(math.Sin(float64(flap*0.5)))*0.2, 0, model, c)
	s.drawPart(k, PartExtra, rl.NewVector3(0.4, 0.35, 2.2), rl.NewVector3(0, 0.3, -4.7), 0, 0.25, float32(math.Sin(float64(flap*0.5)))*0.4, 0, model, c)
	for _, sg := range []float32{-1, 1} {
		// Wings pivot at the shoulder and flap about the body axis.
		s.drawPart(k, PartArmR, rl.NewVector3(3.2, 0.12, 2.2), rl.NewVector3(sg*0.6, 0.45, 0.4), 0, 0, 0, sg*(0.3+wing), model, c)
		s.drawPart(k, PartArmR, rl.NewVector3(2.4, 0.1, 1.6), rl.NewVector3(sg*3.6, 0.45+sg*0, 0.3), 0, 0, 0, sg*(0.3+wing)*1.6, model, c)
		s.drawPart(k, PartLegL, rl.NewVector3(0.3, 0.8, 0.3), rl.NewVector3(sg*0.45, -0.5, -0.6), 0.5, 1.0, 0, 0, model, c)
		s.drawPart(k, PartLegL, rl.NewVector3(0.25, 0.6, 0.25), rl.NewVector3(sg*0.4, -0.5, 1.0), 0.5, 1.1, 0, 0, model, c)
	}
	if p.Flash > 0 {
		rl.DrawCubeV(p.Pos, rl.NewVector3(3, 2, 7), rl.Fade(rl.White, p.Flash*0.4))
	}
}

func (g *Game) drawDragons(cam rl.Camera3D) {
	for _, d := range g.Dragons {
		if d.State == DragonDead {
			continue
		}
		pos := Near(d.Pos, cam.Position)
		if rl.Vector3Distance(pos, cam.Position) > 200 {
			continue
		}
		yaw := float32(math.Atan2(float64(d.Vel.X), float64(d.Vel.Z)))
		pitch := float32(0)
		if l := rl.Vector3Length(d.Vel); l > 0.1 {
			pitch = -float32(math.Asin(float64(d.Vel.Y / l)))
		}
		pose := Pose{Pos: pos, Yaw: yaw, Pitch: pitch * 0.5, Scale: 1.4, Lum: g.World.Luminance(pos, g.Sky.Light()), Alpha: 1, Flash: d.Flash}
		if d.State == DragonDying {
			pose.Death = clamp(d.DeathT*0.5, 0, 1)
		}
		if d.FireCD > 3.2 {
			pose.Swing = 1 // jaws open as it breathes
		}
		skins.DrawDragon(g.World, &pose, d.Kind, d.Flap)
		if d.HP < d.MaxHP && d.State != DragonDying {
			top := pos.Y + 3
			frac := float32(d.HP) / float32(d.MaxHP)
			rl.DrawCubeV(rl.NewVector3(pos.X, top, pos.Z), rl.NewVector3(4, 0.12, 0.12), rl.NewColor(0, 0, 0, 180))
			rl.DrawCubeV(rl.NewVector3(pos.X-(1-frac)*2, top, pos.Z), rl.NewVector3(frac*4, 0.14, 0.14), rl.Red)
		}
	}
	for i := range g.Fireballs {
		f := &g.Fireballs[i]
		pos := Near(f.Pos, cam.Position)
		r := float32(0.45)
		if f.Kind == DragonEarth {
			r = 0.7
		}
		rl.DrawSphere(pos, r, fireColor(f.Kind))
		rl.DrawSphere(pos, r+0.25, rl.Fade(fireColor(f.Kind), 0.35))
	}
}

// dragonHint names the nearest living dragon for the HUD.
func (g *Game) dragonHint() string {
	var best *Dragon
	bd := float32(1e9)
	for _, d := range g.Dragons {
		if d.State == DragonDead {
			continue
		}
		if dist := WrapDist(d.Pos, g.Player.Pos); dist < bd {
			best, bd = d, dist
		}
	}
	if best == nil {
		return ""
	}
	if bd > 120 {
		return ""
	}
	state := "circling"
	if best.State == DragonAttack {
		state = "HUNTING YOU"
	}
	return fmt.Sprintf("%s %dm, %s", best.Name, int(bd), state)
}
