package main

import (
	"os"
	"path/filepath"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// BLOCKWORLD_SHOWCASE=dir stages the building blocks, trees, crops and
// dragons in front of the camera and saves pictures of them into dir, so a
// change to how any of them is drawn can be checked by eye.

// saveShot writes the frame on screen to a PNG. Unlike raylib's own
// TakeScreenshot it is correct on HiDPI displays.
func saveShot(path string) {
	img := rl.LoadImageFromScreen()
	if img == nil {
		return
	}
	rl.ExportImage(*img, path)
	rl.UnloadImage(img)
}

// showcaseStep drives the session; it returns true when finished.
func (g *Game) showcaseStep(frame int, dir string) bool {
	p := g.Player
	look := func(pos rl.Vector3, yaw, pitch float32) {
		g.showCam = [5]float32{pos.X, pos.Y, pos.Z, yaw, pitch}
	}
	// The camera is pinned every frame, so a nudge of the real mouse cannot spoil a picture.
	defer func() {
		if g.showCam != [5]float32{} && p != nil {
			settings.Creative, p.Flying = true, true
			p.Pos = rl.NewVector3(g.showCam[0], g.showCam[1], g.showCam[2])
			p.Yaw, p.Pitch = g.showCam[3], g.showCam[4]
			p.VelY = 0
		}
	}()
	switch frame {
	case 20:
		g.Reset()
		g.Sky.Raining, g.Sky.Rain = false, 0
		g.ShowHelp = false
		g.Enemies = nil
		g.buildShowcase()
	case 21, 150, 280:
		g.Sky.T = 0.25 // keep it mid-morning
	case 140:
		saveShot(filepath.Join(dir, "showcase_wide.png"))
		x, y, z := g.showcaseOrigin()
		look(rl.NewVector3(float32(x)+0.5, float32(y)+1.2, float32(z)+5.5), 0, -0.12)
	case 200:
		saveShot(filepath.Join(dir, "showcase_doors.png"))
		x, y, z := g.showcaseOrigin()
		look(rl.NewVector3(float32(x)-7, float32(y)+3.5, float32(z)+4), 0, -0.45)
	case 260:
		saveShot(filepath.Join(dir, "showcase_pen.png"))
		x, y, z := g.showcaseOrigin()
		look(rl.NewVector3(float32(x)+0.5, float32(y)+2.4, float32(z)-1.5), 0, -0.42)
	case 290:
		saveShot(filepath.Join(dir, "showcase_home.png"))
		x, y, z := g.showcaseOrigin()
		look(rl.NewVector3(float32(x)-2, float32(y)+2.3, float32(z)+3.4), 0, -0.3)
	case 305:
		saveShot(filepath.Join(dir, "showcase_living.png"))
		x, y, z := g.showcaseOrigin()
		look(rl.NewVector3(float32(x)+11, float32(y)+6, float32(z)+1), 0, -0.05)
	case 320:
		saveShot(filepath.Join(dir, "showcase_trees.png"))
		// The four dragons in a row above the scene.
		x, y, z := g.showcaseOrigin()
		look(rl.NewVector3(float32(x), float32(y)+16, float32(z)-6), 0, 0.1)
		for i, d := range g.Dragons {
			d.Roost = rl.NewVector3(float32(x)+float32(i-1)*400, 0, float32(z)+400) // far away: they hold still
			d.State = DragonDead
			d.Pos = rl.NewVector3(float32(x)+(float32(i)-1.5)*11, float32(y)+18, float32(z)+16)
			d.Vel = rl.NewVector3(-6, 0, 0)
		}
	case 322:
		for _, d := range g.Dragons {
			d.State = DragonCircle
		}
	case 324:
		saveShot(filepath.Join(dir, "showcase_dragons.png"))
		// One dragon close up, three-quarter view.
		x, y, z := g.showcaseOrigin()
		look(rl.NewVector3(float32(x), float32(y)+17, float32(z)-2), 0, 0.05)
		for i, d := range g.Dragons {
			d.State = DragonDead
			d.Pos = rl.NewVector3(float32(x)+float32(i-1)*60, float32(y)+18, float32(z)+16)
			d.Vel = rl.NewVector3(-5, 0, -3)
		}
		g.showKind = (g.showKind + 1) % len(g.Dragons)
		g.Dragons[g.showKind].Pos.X = float32(x)
	case 326, 332, 338, 344:
		for _, d := range g.Dragons {
			d.State = DragonCircle
		}
	case 328, 334, 340, 346:
		saveShot(filepath.Join(dir, "creature_"+g.Dragons[g.showKind].Name+".png"))
		x, y, z := g.showcaseOrigin()
		for i, d := range g.Dragons {
			d.State = DragonDead
			d.Pos = rl.NewVector3(float32(x)+float32(i-1)*60+200, float32(y)+18, float32(z)+16)
		}
		g.showKind = (g.showKind + 1) % len(g.Dragons)
		g.Dragons[g.showKind].Pos.X = float32(x)
		g.Dragons[g.showKind].Vel = rl.NewVector3(-5, 0, -3)
	case 350:
		g.parade([]AnimalKind{AnimalPig, AnimalCow, AnimalSheep, AnimalWolf, AnimalCat, AnimalHorse}, 2.6, 5)
		x, y, z := g.showcaseOrigin()
		look(rl.NewVector3(float32(x), float32(y)+1.0, float32(z)-2.5), 0, -0.05)
	case 380:
		saveShot(filepath.Join(dir, "creature_farm.png"))
		g.parade([]AnimalKind{AnimalKangaroo, AnimalEmu, AnimalKoala, AnimalWombat, AnimalPlatypus, AnimalCrocodile}, 2.8, 5)
	case 410:
		saveShot(filepath.Join(dir, "creature_outback.png"))
		g.parade([]AnimalKind{AnimalCompy, AnimalRaptor, AnimalDino, AnimalBronto}, 6, 12)
		x, y, z := g.showcaseOrigin()
		look(rl.NewVector3(float32(x), float32(y)+3, float32(z)-6), 0, 0)
	case 440:
		saveShot(filepath.Join(dir, "creature_dinos.png"))
		return true
	}
	if frame >= 350 {
		// Hold the parade still and the weather fair.
		g.Enemies = nil
		g.Sky.T = 0.25
		for _, a := range g.Animals {
			a.WanderT, a.Walking, a.Flee = 99, false, 0
		}
	}
	if frame == 21 {
		x, y, z := g.showcaseOrigin()
		look(rl.NewVector3(float32(x)+0.5, float32(y)+7, float32(z)-5), 0, -0.4)
	}
	return false
}

// showcaseOrigin is the near middle of the stage, at ground level.
func (g *Game) showcaseOrigin() (x, y, z int) {
	return floorI(g.Spawn.X), g.showY, floorI(g.Spawn.Z) + 6
}

// buildShowcase levels a stage beside spawn and sets everything out on it.
func (g *Game) buildShowcase() {
	w := g.World
	sx, sz := floorI(g.Spawn.X), floorI(g.Spawn.Z)+6
	g.showY = max(w.SurfaceY(sx, sz), seaLevel+2)
	y := g.showY
	for dz := -8; dz <= 26; dz++ {
		for dx := -16; dx <= 16; dx++ {
			for yy := y - 3; yy < y+24; yy++ {
				b := Air
				switch {
				case yy < y-1:
					b = Dirt
				case yy == y-1:
					b = Grass
				}
				w.Set(sx+dx, yy, sz+dz, b)
			}
		}
	}
	set := func(dx, dy, dz int, b Block) { w.Set(sx+dx, y+dy, sz+dz, b) }
	// A wall with a closed door, an open door and a window.
	for dx := -4; dx <= 4; dx++ {
		for dy := 0; dy < 3; dy++ {
			set(dx, dy, 10, Planks)
		}
	}
	set(-3, 1, 10, Glass)
	set(3, 1, 10, Glass)
	set(-1, 0, 10, Air)
	set(-1, 1, 10, Air)
	set(1, 0, 10, Air)
	set(1, 1, 10, Air)
	w.SetDoor(sx-1, y, sz+10, false)
	w.SetDoor(sx+1, y, sz+10, true)
	// Crop rows in front of the wall.
	for i, b := range []Block{Seeds, WheatGrowing, Wheat, CarrotGrowing, CarrotRipe, BeetrootRipe, PumpkinSeeds, PumpkinStem, MelonStem} {
		set(-4+i, 0, 8, b)
		set(-4+i, 0, 7, b)
	}
	// Trapdoors: one closed over a pit, one open beside a block.
	set(-6, -1, 7, Air)
	set(-6, -2, 7, Air)
	set(-6, -1, 7, TrapdoorClosed)
	set(-6, 0, 6, TrapdoorClosed)
	set(6, 0, 7, Planks)
	set(6, 0, 6, TrapdoorOpen)
	set(7, 0, 7, Torch)
	// A fence pen with a pumpkin and a watermelon in it, and a pond poured from a bucket.
	for dx := -12; dx <= -7; dx++ {
		for dz := 10; dz <= 14; dz++ {
			if dx == -12 || dx == -7 || dz == 10 || dz == 14 {
				set(dx, 0, dz, Fence)
			}
		}
	}
	set(-10, 0, 12, Pumpkin)
	set(-9, 0, 12, Melon)
	set(-8, 0, 12, Pumpkin)
	for dx := -11; dx <= -9; dx++ {
		for dz := 5; dz <= 6; dz++ {
			set(dx, -1, dz, Air)
			g.applyBucket(sx+dx, y-1, sz+dz, Water)
		}
	}
	// Homewares: a long table with chairs and a vase, a bunk bed, and every wild flower.
	set(-1, 0, 3, Table)
	set(0, 0, 3, Table)
	set(0, 1, 3, Vase)
	set(-1, 0, 2, Chair)
	set(0, 0, 4, Chair)
	set(1, 0, 3, Chair)
	set(3, 0, 4, Planks)
	set(3, 1, 4, Planks)
	w.SetBunk(sx+3, y, sz+3)
	for i, f := range flowerKinds {
		set(-5+i, 0, 1, f)
	}
	set(-3, 0, 4, Chair) // on its own: back to nothing in particular
	// A living room: a wall with a fireplace, bookshelves, a painting and a clock,
	// a two-seat sofa on a rug facing the fire, and a lamp beside it.
	for dx := -4; dx <= 0; dx++ {
		for dy := 0; dy <= 2; dy++ {
			set(dx, dy, 8, Planks)
		}
	}
	set(-2, 0, 8, Fireplace)
	set(-4, 0, 7, Bookshelf)
	set(-4, 1, 7, Bookshelf)
	set(-3, 1, 7, Painting)
	set(-1, 1, 7, Clock)
	for i, r := range []Block{RugRed, Rug, RugBlue} {
		set(-3+i, 0, 6, r)
	}
	set(-3, 0, 5, Sofa)
	set(-2, 0, 5, Sofa)
	set(-1, 0, 5, Lamp)
	// One of every tree.
	tree := func(dx, dz int, place func(x, y, z int, set func(x, y, z int, b Block))) {
		place(sx+dx-originX, y, sz+dz-originZ, func(lx, yy, lz int, b Block) { w.Set(lx+originX, yy, lz+originZ, b) })
	}
	tree(6, 14, w.placeAcacia)
	tree(12, 12, w.placePalm)
	tree(10, 20, w.placeCherry)
	tree(3, 20, w.placeTree)
	tree(-3, 20, w.placeBirch)
	tree(14, 20, w.placeSpruce)
	for _, d := range [][2]int{{8, 9}, {9, 10}, {11, 16}} {
		set(d[0], 0, d[1], []Block{AcaciaSapling, PalmSapling, CherrySapling}[(d[0]+d[1])%3])
	}
}

// parade replaces the animals with a row of the given kinds on the open
// front of the stage, standing still at an angle to the camera.
func (g *Game) parade(kinds []AnimalKind, gap, dist float32) {
	x, y, z := g.showcaseOrigin()
	g.Animals = nil
	for i, k := range kinds {
		px := float32(x) + (float32(i)-float32(len(kinds)-1)/2)*gap
		a := NewAnimal(rl.NewVector3(px, float32(y), float32(z)-2.5+dist), k)
		a.Heading = rl.Vector3Normalize(rl.NewVector3(0.8, 0, -0.6))
		a.WanderT = 99
		a.Walking = false
		if k == AnimalCat || k == AnimalWolf || k == AnimalHorse {
			a.Tamed = true
		}
		g.Animals = append(g.Animals, a)
	}
}

func showcaseDir() string { return os.Getenv("BLOCKWORLD_SHOWCASE") }
