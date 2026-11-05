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
		settings.Creative, p.Flying = true, true
		p.Pos, p.Yaw, p.Pitch = pos, yaw, pitch
		p.Held = Item{Kind: ItemPickaxe}
	}
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
		return true
	}
	if frame > 20 && frame < 140 {
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

func showcaseDir() string { return os.Getenv("BLOCKWORLD_SHOWCASE") }
