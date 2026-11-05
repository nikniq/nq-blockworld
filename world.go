package main

import (
	"math"
	"math/rand"
	"runtime"
	"sort"
	"sync"
	"time"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// The world is a fixed voxel volume. Block coordinates run from originX/originZ
// (inclusive) to originX+worldW / originZ+worldD (exclusive) on X/Z and 0..worldH on Y.
const (
	worldH     = 96
	chunkSize  = 16
	groundBase = 32              // the surface terrain starts this far up; below is the deep
	seaLevel   = groundBase + 10 // every air cell at or below this height is filled with water
	deepTop    = groundBase - 4  // deep stone, big caverns and the richest ores lie below here
)

// World footprint. Chosen per world (settings), stored in saves and sent to
// joining players; setWorldSize must run before a world is created or loaded.
var (
	worldW    = 384
	worldD    = 384
	originX   = -worldW / 2
	originZ   = -worldD / 2
	areaScale = (worldW * worldD) / (96 * 96) // feature counts were tuned on a 96x96 map
)

// worldSizes are the choices offered in the menu (multiples of 48 so the noise tiles).
var worldSizes = []int{192, 384, 576}
var worldSizeNames = []string{"Normal (192)", "Large (384)", "Huge (576)"}

func setWorldSize(n int) {
	if n < chunkSize || n%48 != 0 {
		n = 384
	}
	worldW, worldD = n, n
	originX, originZ = -n/2, -n/2
	areaScale = max(1, (n*n)/(96*96))
}

type Block uint8

const (
	Air Block = iota
	Grass
	Dirt
	Stone
	Cobble
	Sand
	Gravel
	Snow
	Log
	Leaves
	Planks
	StoneBrick
	Glass
	Water
	CoalOre
	IronOre
	GoldOre
	DiamondOre
	GoldBlock
	Torch
	TNT
	Meat
	CookedMeat
	Apple
	Lava
	Wool
	Bed
	Sapling
	Ladder
	BirchLog
	SpruceLeaves
	Cactus
	TallGrass
	Flower
	Leather
	Seeds
	WheatGrowing
	Wheat
	WheatItem
	Bread
	Bow
	ArrowItem
	DoorClosed
	DoorOpen
	Beacon
	BeaconLit
	Fish
	Rod
	Boat
	FlagPost
	FlagItem
	ChestBlock
	DeepStone
	Mud
	Meteorite
	FruitLeaves
	Glowshroom
	Amethyst
	RedSand
	EucLog
	EucLeaves
	DeadBush
	Spawner
	Crate
	Bedrock
	Fence
	Bucket
	WaterBucket
	DoorTop
	DoorTopOpen
	TrapdoorClosed
	TrapdoorOpen
	AcaciaLog
	AcaciaLeaves
	AcaciaSapling
	PalmLog
	PalmLeaves
	PalmSapling
	Coconut
	CherryLog
	CherryLeaves
	CherrySapling
	Carrot
	CarrotGrowing
	CarrotRipe
	Beetroot
	BeetrootGrowing
	BeetrootRipe
	PumpkinSeeds
	PumpkinStem
	Pumpkin
	MelonSeeds
	MelonStem
	Melon
	MelonSlice
	PumpkinPie
	numBlocks
)

// Pickaxe tiers. Tier 0 is bare hands.
const (
	TierHand = iota
	TierWood
	TierStone
	TierIron
	TierDiamond
)

var tierNames = [...]string{"", "Wooden", "Stone", "Iron", "Diamond"}

type blockInfo struct {
	Name              string
	Top, Side, Bottom rl.Color // base tints for the generated textures, the UI and the minimap
	Pat               [3]texPattern
	Spot              rl.Color // ore fleck colour
	MineTime          float32  // seconds to break by hand; < 0 means unbreakable
	Hard              bool     // pickaxe tier speeds up mining
	MinTier           int      // pickaxe tier required to harvest at all
	Drops             Block
	Trans             bool           // rendered in the translucent pass; does not occlude neighbours
	Solid             bool           // blocks movement
	Tiny              bool           // drawn as a small box inside the cell (torches); never occludes
	Emit              uint8          // block light emitted, 0..15
	Food              int            // health restored when eaten (an item, not a placeable block)
	Item              bool           // a crafting material that cannot be placed
	Box               *[2][3]float32 // extents of a Tiny block within its cell (nil: torch size)
	Cross             bool           // drawn as two crossed quads (plants) instead of a box
}

var (
	torchBox      = [2][3]float32{{0.4375, 0, 0.4375}, {0.5625, 0.625, 0.5625}}
	bedBox        = [2][3]float32{{0, 0, 0}, {1, 0.5, 1}}
	saplingBox    = [2][3]float32{{0.3, 0, 0.3}, {0.7, 0.8, 0.7}}
	ladderBox     = [2][3]float32{{0.25, 0, 0.25}, {0.75, 1, 0.75}}
	plantBox      = [2][3]float32{{0.15, 0, 0.15}, {0.85, 0.85, 0.85}}
	cropBox       = [2][3]float32{{0.15, 0, 0.15}, {0.85, 0.3, 0.85}}
	flagBox       = [2][3]float32{{0.44, 0, 0.44}, {0.56, 3, 0.56}}
	doorClosedBox = [2][3]float32{{0, 0, 0.4}, {1, 1, 0.6}}
	doorOpenBox   = [2][3]float32{{0, 0, 0}, {0.2, 1, 1}}
	fencePostBox  = [2][3]float32{{0.375, 0, 0.375}, {0.625, 1.5, 0.625}}
	trapdoorBox   = [2][3]float32{{0, 0, 0}, {1, trapdoorH, 1}}
)

const trapdoorH = 0.1875 // a closed trapdoor is a thin slab at the bottom of its cell

// doorBox orients a door to its doorway: closed, it spans the gap between the
// walls either side; open, it swings back flat against the hinge wall. Door
// tops copy the half below them.
func (w *World) doorBox(lx, y, lz int, b Block) [2][3]float32 {
	if b == DoorTop || b == DoorTopOpen {
		y--
	}
	open := b == DoorOpen || b == DoorTopOpen
	const t = 0.1875
	wallX := w.getLocal(lx-1, y, lz).Opaque() || w.getLocal(lx+1, y, lz).Opaque()
	wallZ := w.getLocal(lx, y, lz-1).Opaque() || w.getLocal(lx, y, lz+1).Opaque()
	alongX := wallX || !wallZ // the door spans X when its walls are on the X sides
	if !open {
		if alongX {
			return [2][3]float32{{0, 0, 0.5 - t/2}, {1, 1, 0.5 + t/2}}
		}
		return [2][3]float32{{0.5 - t/2, 0, 0}, {0.5 + t/2, 1, 1}}
	}
	if alongX {
		if w.getLocal(lx+1, y, lz).Opaque() && !w.getLocal(lx-1, y, lz).Opaque() {
			return [2][3]float32{{1 - t, 0, 0}, {1, 1, 1}}
		}
		return [2][3]float32{{0, 0, 0}, {t, 1, 1}}
	}
	if w.getLocal(lx, y, lz+1).Opaque() && !w.getLocal(lx, y, lz-1).Opaque() {
		return [2][3]float32{{0, 0, 1 - t}, {1, 1, 1}}
	}
	return [2][3]float32{{0, 0, 0}, {1, 1, t}}
}

// openTrapdoorBox stands the trapdoor up against whichever side has a wall.
func (w *World) openTrapdoorBox(lx, y, lz int) [2][3]float32 {
	const t = trapdoorH
	switch {
	case w.getLocal(lx, y, lz-1).Opaque():
		return [2][3]float32{{0, 0, 0}, {1, 1, t}}
	case w.getLocal(lx, y, lz+1).Opaque():
		return [2][3]float32{{0, 0, 1 - t}, {1, 1, 1}}
	case w.getLocal(lx-1, y, lz).Opaque():
		return [2][3]float32{{0, 0, 0}, {t, 1, 1}}
	case w.getLocal(lx+1, y, lz).Opaque():
		return [2][3]float32{{1 - t, 0, 0}, {1, 1, 1}}
	}
	return [2][3]float32{{0, 0, 0}, {1, 1, t}}
}

// isDoor reports whether a block is any half of a door, open or closed.
func isDoor(b Block) bool {
	return b == DoorClosed || b == DoorOpen || b == DoorTop || b == DoorTopOpen
}

// SetDoor writes both halves of a door whose bottom is at y.
func (w *World) SetDoor(x, y, z int, open bool) {
	lo, hi := DoorClosed, DoorTop
	if open {
		lo, hi = DoorOpen, DoorTopOpen
	}
	w.Set(x, y, z, lo)
	w.Set(x, y+1, z, hi)
}

// setDoorLocal is SetDoor for world generation.
func (w *World) setDoorLocal(lx, y, lz int) {
	w.setLocal(lx, y, lz, DoorClosed)
	w.setLocal(lx, y+1, lz, DoorTop)
}

// DoorBottom returns the y of the lower half of the door at a cell.
func (w *World) DoorBottom(x, y, z int) int {
	if b := w.Get(x, y, z); b == DoorTop || b == DoorTopOpen {
		return y - 1
	}
	return y
}

// cropNext is the next growth stage of a crop block, or Air if it does not grow.
func cropNext(b Block) Block {
	switch b {
	case Seeds:
		return WheatGrowing
	case WheatGrowing:
		return Wheat
	case CarrotGrowing:
		return CarrotRipe
	case BeetrootGrowing:
		return BeetrootRipe
	case PumpkinSeeds:
		return PumpkinStem
	case MelonSeeds:
		return MelonStem
	}
	return Air
}

// NeedsSoil reports whether a block can only be planted on grass or dirt.
func (b Block) NeedsSoil() bool {
	switch b {
	case Sapling, AcaciaSapling, PalmSapling, CherrySapling, Seeds, CarrotGrowing, BeetrootGrowing, PumpkinSeeds, MelonSeeds:
		return true
	}
	return false
}

// Plants returns the crop a food item grows into when planted, or Air.
func (b Block) Plants() Block {
	switch b {
	case Carrot:
		return CarrotGrowing
	case Beetroot:
		return BeetrootGrowing
	}
	return Air
}

// IsLeaves reports whether a block is any tree's foliage.
func (b Block) IsLeaves() bool {
	switch b {
	case Leaves, SpruceLeaves, EucLeaves, FruitLeaves, AcaciaLeaves, PalmLeaves, CherryLeaves:
		return true
	}
	return false
}

// fenceBoxes is the post plus a pair of rails toward every neighbour a fence
// joins: another fence or a full block.
func (w *World) fenceBoxes(lx, y, lz int) [][2][3]float32 {
	out := [][2][3]float32{fencePostBox}
	join := func(b Block) bool { return b == Fence || (b.Opaque() && blocks[b].Solid) }
	const hw = 0.0625
	for _, rail := range [][2]float32{{0.45, 0.6}, {1.05, 1.2}} {
		if join(w.getLocal(lx, y, lz-1)) {
			out = append(out, [2][3]float32{{0.5 - hw, rail[0], 0}, {0.5 + hw, rail[1], 0.375}})
		}
		if join(w.getLocal(lx, y, lz+1)) {
			out = append(out, [2][3]float32{{0.5 - hw, rail[0], 0.625}, {0.5 + hw, rail[1], 1}})
		}
		if join(w.getLocal(lx-1, y, lz)) {
			out = append(out, [2][3]float32{{0, rail[0], 0.5 - hw}, {0.375, rail[1], 0.5 + hw}})
		}
		if join(w.getLocal(lx+1, y, lz)) {
			out = append(out, [2][3]float32{{0.625, rail[0], 0.5 - hw}, {1, rail[1], 0.5 + hw}})
		}
	}
	return out
}

// Biomes decide the surface blocks and vegetation of a column.
type Biome uint8

const (
	BiomePlains Biome = iota
	BiomeForest
	BiomeDesert
	BiomeTaiga
	BiomeOutback
	BiomeSwamp
)

// ladderBox leans a ladder against the first solid wall beside it: a thin
// panel, not a block. Without a wall it stands as a thin panel across the cell.
func (w *World) ladderBox(lx, y, lz int) [2][3]float32 {
	const t = 0.12
	switch {
	case w.getLocal(lx, y, lz-1).Opaque():
		return [2][3]float32{{0.05, 0, 0.02}, {0.95, 1, 0.02 + t}}
	case w.getLocal(lx, y, lz+1).Opaque():
		return [2][3]float32{{0.05, 0, 0.98 - t}, {0.95, 1, 0.98}}
	case w.getLocal(lx-1, y, lz).Opaque():
		return [2][3]float32{{0.02, 0, 0.05}, {0.02 + t, 1, 0.95}}
	case w.getLocal(lx+1, y, lz).Opaque():
		return [2][3]float32{{0.98 - t, 0, 0.05}, {0.98, 1, 0.95}}
	}
	return [2][3]float32{{0.05, 0, 0.44}, {0.95, 1, 0.56}}
}

// TinyBox returns the extents of a Tiny block.
func (b Block) TinyBox() [2][3]float32 {
	if bx := blocks[b].Box; bx != nil {
		return *bx
	}
	return torchBox
}

func col(r, g, b uint8) rl.Color { return rl.NewColor(r, g, b, 255) }

var blocks = [numBlocks]blockInfo{
	Air: {Name: "Air", MineTime: -1},
	Grass: {Name: "Grass", Top: col(112, 176, 66), Side: col(124, 92, 58), Bottom: col(124, 92, 58),
		Pat: [3]texPattern{PatGrassTop, PatGrassSide, PatNoise}, MineTime: 0.6, Drops: Dirt, Solid: true},
	Dirt: {Name: "Dirt", Top: col(124, 92, 58), Side: col(124, 92, 58), Bottom: col(124, 92, 58),
		Pat: [3]texPattern{PatNoise, PatNoise, PatNoise}, MineTime: 0.5, Drops: Dirt, Solid: true},
	Stone: {Name: "Stone", Top: col(128, 128, 130), Side: col(125, 125, 128), Bottom: col(120, 120, 124),
		Pat: [3]texPattern{PatNoise, PatNoise, PatNoise}, MineTime: 3.0, Hard: true, Drops: Cobble, Solid: true},
	Cobble: {Name: "Cobblestone", Top: col(118, 118, 120), Side: col(118, 118, 120), Bottom: col(110, 110, 112),
		Pat: [3]texPattern{PatCobble, PatCobble, PatCobble}, MineTime: 3.0, Hard: true, Drops: Cobble, Solid: true},
	Sand: {Name: "Sand", Top: col(222, 208, 150), Side: col(216, 202, 144), Bottom: col(208, 194, 138),
		Pat: [3]texPattern{PatSand, PatSand, PatSand}, MineTime: 0.5, Drops: Sand, Solid: true},
	Gravel: {Name: "Gravel", Top: col(132, 126, 122), Side: col(132, 126, 122), Bottom: col(126, 120, 116),
		Pat: [3]texPattern{PatGravel, PatGravel, PatGravel}, MineTime: 0.6, Drops: Gravel, Solid: true},
	Snow: {Name: "Snow", Top: col(240, 244, 250), Side: col(124, 92, 58), Bottom: col(124, 92, 58),
		Pat: [3]texPattern{PatSand, PatSnowSide, PatNoise}, MineTime: 0.5, Drops: Dirt, Solid: true},
	Log: {Name: "Oak Log", Top: col(172, 140, 86), Side: col(104, 78, 46), Bottom: col(172, 140, 86),
		Pat: [3]texPattern{PatLogTop, PatLogSide, PatLogTop}, MineTime: 1.5, Drops: Log, Solid: true},
	Leaves: {Name: "Leaves", Top: col(58, 132, 48), Side: col(54, 124, 46), Bottom: col(48, 110, 42),
		Pat: [3]texPattern{PatLeaves, PatLeaves, PatLeaves}, MineTime: 0.3, Drops: Leaves, Solid: true},
	Planks: {Name: "Oak Planks", Top: col(186, 148, 88), Side: col(180, 142, 82), Bottom: col(172, 134, 76),
		Pat: [3]texPattern{PatPlanks, PatPlanks, PatPlanks}, MineTime: 1.2, Drops: Planks, Solid: true},
	StoneBrick: {Name: "Stone Bricks", Top: col(122, 122, 126), Side: col(122, 122, 126), Bottom: col(116, 116, 120),
		Pat: [3]texPattern{PatBricks, PatBricks, PatBricks}, MineTime: 3.0, Hard: true, Drops: StoneBrick, Solid: true},
	Glass: {Name: "Glass", Top: col(200, 230, 240), Side: col(200, 230, 240), Bottom: col(200, 230, 240),
		Pat: [3]texPattern{PatGlass, PatGlass, PatGlass}, MineTime: 0.4, Drops: Glass, Trans: true, Solid: true},
	Water: {Name: "Water", Top: col(40, 90, 200), Side: col(40, 90, 200), Bottom: col(40, 90, 200),
		Pat: [3]texPattern{PatWater, PatWater, PatWater}, MineTime: -1, Trans: true},
	CoalOre: {Name: "Coal Ore", Top: col(125, 125, 128), Side: col(125, 125, 128), Bottom: col(125, 125, 128),
		Pat: [3]texPattern{PatOre, PatOre, PatOre}, Spot: col(28, 28, 30), MineTime: 4.5, Hard: true, Drops: CoalOre, Solid: true},
	IronOre: {Name: "Iron Ore", Top: col(125, 125, 128), Side: col(125, 125, 128), Bottom: col(125, 125, 128),
		Pat: [3]texPattern{PatOre, PatOre, PatOre}, Spot: col(214, 170, 140), MineTime: 4.5, Hard: true, MinTier: TierStone, Drops: IronOre, Solid: true},
	GoldOre: {Name: "Gold Ore", Top: col(125, 125, 128), Side: col(125, 125, 128), Bottom: col(125, 125, 128),
		Pat: [3]texPattern{PatOre, PatOre, PatOre}, Spot: col(250, 214, 70), MineTime: 4.5, Hard: true, MinTier: TierIron, Drops: GoldOre, Solid: true},
	DiamondOre: {Name: "Diamond Ore", Top: col(125, 125, 128), Side: col(125, 125, 128), Bottom: col(125, 125, 128),
		Pat: [3]texPattern{PatOre, PatOre, PatOre}, Spot: col(96, 236, 232), MineTime: 4.5, Hard: true, MinTier: TierIron, Drops: DiamondOre, Solid: true},
	GoldBlock: {Name: "Gold Block", Top: col(248, 210, 60), Side: col(244, 200, 52), Bottom: col(236, 190, 48),
		Pat: [3]texPattern{PatGold, PatGold, PatGold}, MineTime: 4.5, Hard: true, Drops: GoldBlock, Solid: true},
	Torch: {Name: "Torch", Top: col(255, 200, 60), Side: col(110, 80, 40), Bottom: col(90, 65, 30),
		Pat: [3]texPattern{PatSand, PatTorch, PatNoise}, MineTime: 0.05, Drops: Torch, Tiny: true, Emit: 15},
	TNT: {Name: "TNT", Top: col(200, 50, 40), Side: col(200, 50, 40), Bottom: col(200, 50, 40),
		Pat: [3]texPattern{PatTNTTop, PatTNT, PatTNTTop}, MineTime: 0.3, Drops: TNT, Solid: true},
	Meat: {Name: "Raw Meat", Top: col(215, 90, 90), Side: col(215, 90, 90), Bottom: col(215, 90, 90),
		Pat: [3]texPattern{PatMeat, PatMeat, PatMeat}, MineTime: 0.1, Drops: Meat, Food: 5},
	CookedMeat: {Name: "Cooked Meat", Top: col(150, 95, 60), Side: col(150, 95, 60), Bottom: col(150, 95, 60),
		Pat: [3]texPattern{PatMeat, PatMeat, PatMeat}, MineTime: 0.1, Drops: CookedMeat, Food: 10},
	Apple: {Name: "Apple", Top: col(210, 40, 40), Side: col(210, 40, 40), Bottom: col(210, 40, 40),
		Pat: [3]texPattern{PatApple, PatApple, PatApple}, MineTime: 0.1, Drops: Apple, Food: 4},
	Lava: {Name: "Lava", Top: col(240, 110, 20), Side: col(230, 95, 15), Bottom: col(200, 80, 10),
		Pat: [3]texPattern{PatLava, PatLava, PatLava}, MineTime: -1, Emit: 15},
	Wool: {Name: "Wool", Top: col(235, 235, 230), Side: col(228, 228, 222), Bottom: col(220, 220, 215),
		Pat: [3]texPattern{PatWool, PatWool, PatWool}, MineTime: 0.6, Drops: Wool, Solid: true},
	Bed: {Name: "Bed", Top: col(190, 40, 40), Side: col(150, 110, 70), Bottom: col(150, 110, 70),
		Pat: [3]texPattern{PatBedTop, PatPlanks, PatPlanks}, MineTime: 0.3, Drops: Bed, Tiny: true, Box: &bedBox},
	Sapling: {Name: "Oak Sapling", Top: col(70, 140, 50), Side: col(70, 140, 50), Bottom: col(90, 70, 40),
		Pat: [3]texPattern{PatSapling, PatSapling, PatNoise}, MineTime: 0.05, Drops: Sapling, Tiny: true, Box: &saplingBox, Cross: true},
	Ladder: {Name: "Ladder", Top: col(160, 120, 70), Side: col(160, 120, 70), Bottom: col(160, 120, 70),
		Pat: [3]texPattern{PatLadder, PatLadder, PatLadder}, MineTime: 0.3, Drops: Ladder, Tiny: true, Box: &ladderBox},
	BirchLog: {Name: "Birch Log", Top: col(200, 190, 150), Side: col(225, 225, 215), Bottom: col(200, 190, 150),
		Pat: [3]texPattern{PatLogTop, PatBirchSide, PatLogTop}, MineTime: 1.5, Drops: BirchLog, Solid: true},
	SpruceLeaves: {Name: "Spruce Leaves", Top: col(40, 90, 50), Side: col(36, 84, 46), Bottom: col(30, 72, 40),
		Pat: [3]texPattern{PatLeaves, PatLeaves, PatLeaves}, MineTime: 0.3, Drops: SpruceLeaves, Solid: true},
	Cactus: {Name: "Cactus", Top: col(90, 150, 60), Side: col(70, 130, 50), Bottom: col(90, 150, 60),
		Pat: [3]texPattern{PatCactusTop, PatCactus, PatCactusTop}, MineTime: 0.4, Drops: Cactus, Solid: true},
	TallGrass: {Name: "Tall Grass", Top: col(100, 170, 60), Side: col(100, 170, 60), Bottom: col(100, 170, 60),
		Pat: [3]texPattern{PatTuft, PatTuft, PatTuft}, MineTime: 0.05, Tiny: true, Box: &plantBox, Cross: true},
	Flower: {Name: "Flower", Top: col(230, 60, 60), Side: col(230, 60, 60), Bottom: col(230, 60, 60),
		Pat: [3]texPattern{PatFlower, PatFlower, PatFlower}, MineTime: 0.05, Drops: Flower, Tiny: true, Box: &plantBox, Cross: true},
	Leather: {Name: "Leather", Top: col(150, 95, 55), Side: col(150, 95, 55), Bottom: col(150, 95, 55),
		Pat: [3]texPattern{PatNoise, PatNoise, PatNoise}, MineTime: 0.1, Drops: Leather, Item: true},
	Seeds: {Name: "Wheat Seeds", Top: col(120, 190, 80), Side: col(120, 190, 80), Bottom: col(120, 190, 80),
		Pat: [3]texPattern{PatCrop0, PatCrop0, PatCrop0}, MineTime: 0.05, Drops: Seeds, Tiny: true, Box: &cropBox, Cross: true},
	WheatGrowing: {Name: "Wheat (growing)", Top: col(110, 180, 70), Side: col(110, 180, 70), Bottom: col(110, 180, 70),
		Pat: [3]texPattern{PatCrop1, PatCrop1, PatCrop1}, MineTime: 0.05, Drops: Seeds, Tiny: true, Box: &plantBox, Cross: true},
	Wheat: {Name: "Wheat", Top: col(210, 180, 70), Side: col(210, 180, 70), Bottom: col(210, 180, 70),
		Pat: [3]texPattern{PatCrop2, PatCrop2, PatCrop2}, MineTime: 0.05, Drops: WheatItem, Tiny: true, Box: &plantBox, Cross: true},
	WheatItem: {Name: "Wheat", Top: col(215, 185, 80), Side: col(215, 185, 80), Bottom: col(215, 185, 80),
		Pat: [3]texPattern{PatCrop2, PatCrop2, PatCrop2}, MineTime: 0.1, Drops: WheatItem, Item: true},
	Bread: {Name: "Bread", Top: col(200, 150, 80), Side: col(200, 150, 80), Bottom: col(200, 150, 80),
		Pat: [3]texPattern{PatBread, PatBread, PatBread}, MineTime: 0.1, Drops: Bread, Food: 6},
	Bow: {Name: "Bow", Top: col(130, 90, 50), Side: col(130, 90, 50), Bottom: col(130, 90, 50),
		Pat: [3]texPattern{PatBow, PatBow, PatBow}, MineTime: 0.1, Drops: Bow, Item: true},
	ArrowItem: {Name: "Arrow", Top: col(190, 170, 130), Side: col(190, 170, 130), Bottom: col(190, 170, 130),
		Pat: [3]texPattern{PatArrow, PatArrow, PatArrow}, MineTime: 0.1, Drops: ArrowItem, Item: true},
	DoorClosed: {Name: "Door", Top: col(150, 110, 65), Side: col(150, 110, 65), Bottom: col(150, 110, 65),
		Pat: [3]texPattern{PatPlanks, PatDoor, PatPlanks}, MineTime: 1.0, Drops: DoorClosed, Solid: true, Tiny: true, Box: &doorClosedBox},
	DoorOpen: {Name: "Door (open)", Top: col(150, 110, 65), Side: col(150, 110, 65), Bottom: col(150, 110, 65),
		Pat: [3]texPattern{PatPlanks, PatDoor, PatPlanks}, MineTime: 1.0, Drops: DoorClosed, Tiny: true, Box: &doorOpenBox},
	Beacon: {Name: "Ancient Beacon", Top: col(90, 200, 220), Side: col(70, 150, 170), Bottom: col(60, 120, 140),
		Pat: [3]texPattern{PatBeacon, PatBeacon, PatBeacon}, MineTime: -1, Solid: true, Emit: 9},
	BeaconLit: {Name: "Lit Beacon", Top: col(190, 240, 255), Side: col(150, 220, 240), Bottom: col(120, 190, 210),
		Pat: [3]texPattern{PatBeacon, PatBeacon, PatBeacon}, MineTime: -1, Solid: true, Emit: 15},
	Fish: {Name: "Fish", Top: col(120, 160, 190), Side: col(120, 160, 190), Bottom: col(120, 160, 190),
		Pat: [3]texPattern{PatFish, PatFish, PatFish}, MineTime: 0.1, Drops: Fish, Food: 5},
	Rod: {Name: "Fishing Rod", Top: col(130, 90, 50), Side: col(130, 90, 50), Bottom: col(130, 90, 50),
		Pat: [3]texPattern{PatRod, PatRod, PatRod}, MineTime: 0.1, Drops: Rod, Item: true},
	Mud: {Name: "Mud", Top: col(70, 60, 45), Side: col(70, 60, 45), Bottom: col(65, 55, 40),
		Pat: [3]texPattern{PatNoise, PatNoise, PatNoise}, MineTime: 0.4, Drops: Mud, Solid: true},
	Meteorite: {Name: "Meteorite", Top: col(60, 55, 60), Side: col(55, 50, 55), Bottom: col(50, 45, 50),
		Pat: [3]texPattern{PatMeteor, PatMeteor, PatMeteor}, MineTime: 4, Hard: true, MinTier: TierStone, Drops: Meteorite, Solid: true, Emit: 4},
	FruitLeaves: {Name: "Apple Tree Leaves", Top: col(58, 132, 48), Side: col(54, 124, 46), Bottom: col(48, 110, 42),
		Pat: [3]texPattern{PatFruit, PatFruit, PatFruit}, MineTime: 0.3, Drops: Leaves, Solid: true},
	Boat: {Name: "Boat", Top: col(150, 110, 65), Side: col(150, 110, 65), Bottom: col(150, 110, 65),
		Pat: [3]texPattern{PatBoat, PatBoat, PatBoat}, MineTime: 0.1, Drops: Boat, Item: true},
	FlagPost: {Name: "Flag", Top: col(120, 90, 50), Side: col(120, 90, 50), Bottom: col(120, 90, 50),
		Pat: [3]texPattern{PatNoise, PatLogSide, PatNoise}, MineTime: 1.5, Drops: FlagItem, Tiny: true, Box: &flagBox, Emit: 5},
	FlagItem: {Name: "Village Flag", Top: col(240, 200, 50), Side: col(240, 200, 50), Bottom: col(240, 200, 50),
		Pat: [3]texPattern{PatFlag, PatFlag, PatFlag}, MineTime: 0.1, Drops: FlagItem, Item: true},
	ChestBlock: {Name: "Chest", Top: col(140, 100, 55), Side: col(135, 95, 50), Bottom: col(120, 85, 45),
		Pat: [3]texPattern{PatCrate, PatCrate, PatCrate}, MineTime: 1.2, Drops: ChestBlock, Solid: true},
	DeepStone: {Name: "Deep Stone", Top: col(70, 72, 80), Side: col(66, 68, 76), Bottom: col(60, 62, 70),
		Pat: [3]texPattern{PatNoise, PatNoise, PatNoise}, MineTime: 4.5, Hard: true, Drops: Cobble, Solid: true},
	Glowshroom: {Name: "Glowshroom", Top: col(120, 200, 230), Side: col(120, 200, 230), Bottom: col(120, 200, 230),
		Pat: [3]texPattern{PatShroom, PatShroom, PatShroom}, MineTime: 0.05, Drops: Glowshroom, Tiny: true, Box: &saplingBox, Cross: true, Emit: 9, Food: 3},
	Amethyst: {Name: "Amethyst", Top: col(170, 110, 230), Side: col(160, 100, 220), Bottom: col(140, 90, 200),
		Pat: [3]texPattern{PatAmethyst, PatAmethyst, PatAmethyst}, MineTime: 3.5, Hard: true, MinTier: TierStone, Drops: Amethyst, Solid: true, Emit: 7},
	RedSand: {Name: "Red Sand", Top: col(190, 95, 50), Side: col(182, 90, 48), Bottom: col(170, 85, 45),
		Pat: [3]texPattern{PatSand, PatSand, PatSand}, MineTime: 0.5, Drops: RedSand, Solid: true},
	EucLog: {Name: "Eucalyptus Log", Top: col(200, 180, 150), Side: col(215, 205, 190), Bottom: col(200, 180, 150),
		Pat: [3]texPattern{PatLogTop, PatBirchSide, PatLogTop}, MineTime: 1.5, Drops: EucLog, Solid: true},
	EucLeaves: {Name: "Eucalyptus Leaves", Top: col(120, 150, 110), Side: col(110, 140, 100), Bottom: col(95, 125, 90),
		Pat: [3]texPattern{PatLeaves, PatLeaves, PatLeaves}, MineTime: 0.3, Drops: EucLeaves, Solid: true},
	DeadBush: {Name: "Dead Bush", Top: col(150, 110, 60), Side: col(150, 110, 60), Bottom: col(150, 110, 60),
		Pat: [3]texPattern{PatTuft, PatTuft, PatTuft}, MineTime: 0.05, Tiny: true, Box: &plantBox, Cross: true},
	Spawner: {Name: "Monster Spawner", Top: col(40, 44, 50), Side: col(40, 44, 50), Bottom: col(40, 44, 50),
		Pat: [3]texPattern{PatSpawner, PatSpawner, PatSpawner}, MineTime: 6, Hard: true, Drops: Air, Solid: true},
	Crate: {Name: "Loot Crate", Top: col(170, 130, 70), Side: col(160, 120, 65), Bottom: col(150, 110, 60),
		Pat: [3]texPattern{PatCrate, PatCrate, PatCrate}, MineTime: 0.8, Drops: Air, Solid: true},
	Bedrock: {Name: "Bedrock", Top: col(70, 70, 76), Side: col(70, 70, 76), Bottom: col(70, 70, 76),
		Pat: [3]texPattern{PatBedrock, PatBedrock, PatBedrock}, MineTime: -1, Drops: Bedrock, Solid: true},
	Fence: {Name: "Fence", Top: col(172, 140, 86), Side: col(160, 124, 72), Bottom: col(150, 115, 65),
		Pat: [3]texPattern{PatLogTop, PatFence, PatLogTop}, MineTime: 1.0, Drops: Fence, Solid: true, Tiny: true, Box: &fencePostBox},
	Bucket: {Name: "Bucket", Top: col(160, 164, 170), Side: col(160, 164, 170), Bottom: col(160, 164, 170),
		Pat: [3]texPattern{PatBucket, PatBucket, PatBucket}, MineTime: 0.1, Drops: Bucket, Item: true},
	WaterBucket: {Name: "Water Bucket", Top: col(160, 164, 170), Side: col(160, 164, 170), Bottom: col(160, 164, 170),
		Pat: [3]texPattern{PatWaterBucket, PatWaterBucket, PatWaterBucket}, MineTime: 0.1, Drops: WaterBucket, Item: true},
	DoorTop: {Name: "Door", Top: col(150, 110, 65), Side: col(150, 110, 65), Bottom: col(150, 110, 65),
		Pat: [3]texPattern{PatPlanks, PatDoorHigh, PatPlanks}, MineTime: 1.0, Drops: Air, Solid: true, Tiny: true, Box: &doorClosedBox},
	DoorTopOpen: {Name: "Door (open)", Top: col(150, 110, 65), Side: col(150, 110, 65), Bottom: col(150, 110, 65),
		Pat: [3]texPattern{PatPlanks, PatDoorHigh, PatPlanks}, MineTime: 1.0, Drops: Air, Tiny: true, Box: &doorOpenBox},
	TrapdoorClosed: {Name: "Trapdoor", Top: col(160, 120, 70), Side: col(150, 110, 65), Bottom: col(140, 100, 60),
		Pat: [3]texPattern{PatTrapdoor, PatPlanks, PatTrapdoor}, MineTime: 1.0, Drops: TrapdoorClosed, Solid: true, Tiny: true, Box: &trapdoorBox},
	TrapdoorOpen: {Name: "Trapdoor (open)", Top: col(160, 120, 70), Side: col(150, 110, 65), Bottom: col(140, 100, 60),
		Pat: [3]texPattern{PatPlanks, PatTrapdoor, PatPlanks}, MineTime: 1.0, Drops: TrapdoorClosed, Tiny: true, Box: &trapdoorBox},
	AcaciaLog: {Name: "Acacia Log", Top: col(190, 110, 60), Side: col(110, 100, 90), Bottom: col(190, 110, 60),
		Pat: [3]texPattern{PatLogTop, PatLogSide, PatLogTop}, MineTime: 1.5, Drops: AcaciaLog, Solid: true},
	AcaciaLeaves: {Name: "Acacia Leaves", Top: col(110, 140, 60), Side: col(104, 134, 56), Bottom: col(90, 118, 50),
		Pat: [3]texPattern{PatLeaves, PatLeaves, PatLeaves}, MineTime: 0.3, Drops: AcaciaLeaves, Solid: true},
	AcaciaSapling: {Name: "Acacia Sapling", Top: col(110, 140, 60), Side: col(110, 140, 60), Bottom: col(90, 70, 40),
		Pat: [3]texPattern{PatSapling, PatSapling, PatNoise}, MineTime: 0.05, Drops: AcaciaSapling, Tiny: true, Box: &saplingBox, Cross: true},
	PalmLog: {Name: "Palm Log", Top: col(170, 140, 100), Side: col(150, 120, 80), Bottom: col(170, 140, 100),
		Pat: [3]texPattern{PatLogTop, PatPalmSide, PatLogTop}, MineTime: 1.5, Drops: PalmLog, Solid: true},
	PalmLeaves: {Name: "Palm Fronds", Top: col(70, 150, 60), Side: col(64, 140, 56), Bottom: col(56, 120, 50),
		Pat: [3]texPattern{PatLeaves, PatLeaves, PatLeaves}, MineTime: 0.3, Drops: PalmLeaves, Solid: true},
	PalmSapling: {Name: "Palm Sapling", Top: col(70, 150, 60), Side: col(70, 150, 60), Bottom: col(90, 70, 40),
		Pat: [3]texPattern{PatSapling, PatSapling, PatNoise}, MineTime: 0.05, Drops: PalmSapling, Tiny: true, Box: &saplingBox, Cross: true},
	Coconut: {Name: "Coconut", Top: col(110, 80, 50), Side: col(110, 80, 50), Bottom: col(110, 80, 50),
		Pat: [3]texPattern{PatCoconut, PatCoconut, PatCoconut}, MineTime: 0.1, Drops: Coconut, Food: 4},
	CherryLog: {Name: "Cherry Log", Top: col(200, 150, 120), Side: col(90, 55, 50), Bottom: col(200, 150, 120),
		Pat: [3]texPattern{PatLogTop, PatLogSide, PatLogTop}, MineTime: 1.5, Drops: CherryLog, Solid: true},
	CherryLeaves: {Name: "Cherry Blossom", Top: col(240, 170, 200), Side: col(236, 160, 195), Bottom: col(220, 140, 180),
		Pat: [3]texPattern{PatLeaves, PatLeaves, PatLeaves}, MineTime: 0.3, Drops: CherryLeaves, Solid: true},
	CherrySapling: {Name: "Cherry Sapling", Top: col(240, 170, 200), Side: col(240, 170, 200), Bottom: col(90, 70, 40),
		Pat: [3]texPattern{PatSapling, PatSapling, PatNoise}, MineTime: 0.05, Drops: CherrySapling, Tiny: true, Box: &saplingBox, Cross: true},
	Carrot: {Name: "Carrot", Top: col(235, 130, 40), Side: col(235, 130, 40), Bottom: col(235, 130, 40),
		Pat: [3]texPattern{PatCarrot, PatCarrot, PatCarrot}, MineTime: 0.1, Drops: Carrot, Food: 3},
	CarrotGrowing: {Name: "Carrots (growing)", Top: col(90, 170, 70), Side: col(90, 170, 70), Bottom: col(90, 170, 70),
		Pat: [3]texPattern{PatCrop1, PatCrop1, PatCrop1}, MineTime: 0.05, Drops: Carrot, Tiny: true, Box: &plantBox, Cross: true},
	CarrotRipe: {Name: "Carrots", Top: col(80, 160, 60), Side: col(80, 160, 60), Bottom: col(80, 160, 60),
		Pat: [3]texPattern{PatCarrotCrop, PatCarrotCrop, PatCarrotCrop}, MineTime: 0.05, Drops: Carrot, Tiny: true, Box: &plantBox, Cross: true},
	Beetroot: {Name: "Beetroot", Top: col(150, 30, 60), Side: col(150, 30, 60), Bottom: col(150, 30, 60),
		Pat: [3]texPattern{PatBeet, PatBeet, PatBeet}, MineTime: 0.1, Drops: Beetroot, Food: 2},
	BeetrootGrowing: {Name: "Beetroot (growing)", Top: col(90, 150, 70), Side: col(90, 150, 70), Bottom: col(90, 150, 70),
		Pat: [3]texPattern{PatCrop1, PatCrop1, PatCrop1}, MineTime: 0.05, Drops: Beetroot, Tiny: true, Box: &plantBox, Cross: true},
	BeetrootRipe: {Name: "Beetroot", Top: col(100, 140, 70), Side: col(100, 140, 70), Bottom: col(100, 140, 70),
		Pat: [3]texPattern{PatBeetCrop, PatBeetCrop, PatBeetCrop}, MineTime: 0.05, Drops: Beetroot, Tiny: true, Box: &plantBox, Cross: true},
	PumpkinSeeds: {Name: "Pumpkin Seeds", Top: col(120, 190, 80), Side: col(120, 190, 80), Bottom: col(120, 190, 80),
		Pat: [3]texPattern{PatCrop0, PatCrop0, PatCrop0}, MineTime: 0.05, Drops: PumpkinSeeds, Tiny: true, Box: &cropBox, Cross: true},
	PumpkinStem: {Name: "Pumpkin Vine", Top: col(100, 160, 60), Side: col(100, 160, 60), Bottom: col(100, 160, 60),
		Pat: [3]texPattern{PatStem, PatStem, PatStem}, MineTime: 0.05, Drops: PumpkinSeeds, Tiny: true, Box: &plantBox, Cross: true},
	Pumpkin: {Name: "Pumpkin", Top: col(220, 130, 40), Side: col(225, 125, 35), Bottom: col(200, 115, 35),
		Pat: [3]texPattern{PatPumpkinTop, PatPumpkin, PatPumpkinTop}, MineTime: 0.8, Drops: Pumpkin, Solid: true},
	MelonSeeds: {Name: "Melon Seeds", Top: col(120, 190, 80), Side: col(120, 190, 80), Bottom: col(120, 190, 80),
		Pat: [3]texPattern{PatCrop0, PatCrop0, PatCrop0}, MineTime: 0.05, Drops: MelonSeeds, Tiny: true, Box: &cropBox, Cross: true},
	MelonStem: {Name: "Melon Vine", Top: col(100, 160, 60), Side: col(100, 160, 60), Bottom: col(100, 160, 60),
		Pat: [3]texPattern{PatStem, PatStem, PatStem}, MineTime: 0.05, Drops: MelonSeeds, Tiny: true, Box: &plantBox, Cross: true},
	Melon: {Name: "Watermelon", Top: col(90, 150, 60), Side: col(80, 140, 55), Bottom: col(90, 150, 60),
		Pat: [3]texPattern{PatMelonTop, PatMelon, PatMelonTop}, MineTime: 0.8, Drops: MelonSlice, Solid: true},
	MelonSlice: {Name: "Melon Slice", Top: col(230, 70, 80), Side: col(230, 70, 80), Bottom: col(230, 70, 80),
		Pat: [3]texPattern{PatMelonSlice, PatMelonSlice, PatMelonSlice}, MineTime: 0.1, Drops: MelonSlice, Food: 2},
	PumpkinPie: {Name: "Pumpkin Pie", Top: col(225, 160, 80), Side: col(225, 160, 80), Bottom: col(225, 160, 80),
		Pat: [3]texPattern{PatPie, PatPie, PatPie}, MineTime: 0.1, Drops: PumpkinPie, Food: 8},
}

// Opaque reports whether a block hides the faces of its neighbours.
func (b Block) Opaque() bool { return b != Air && !blocks[b].Trans && !blocks[b].Tiny }

// lightCost is how much light fades passing through a block; 0 means it is opaque to light.
func (b Block) lightCost() uint8 {
	switch {
	case b == Air || blocks[b].Tiny || b == Glass || b == Lava:
		return 1
	case b == Water || b.IsLeaves():
		return 2
	}
	return 0
}

// Placeable reports whether the player may hold and place this block.
func (b Block) Placeable() bool {
	return b != Air && b != Water && b != Lava && blocks[b].Food == 0 && !blocks[b].Item
}

// Liquid reports whether a block is water or lava: not solid, swimmable, replaceable.
func (b Block) Liquid() bool { return b == Water || b == Lava }

type meshBuf struct {
	mesh   rl.Mesh
	loaded bool
	wobble bool // terrain: block corners are nudged so nothing is perfectly straight
	verts  []float32
	norms  []float32
	uvs    []float32
	cols   []uint8
}

// chunk owns two meshes. Each meshBuf is its own heap object: cgo's pointer
// check scans the whole object around a mesh, so nothing else may live beside it.
// chunk owns one opaque and one translucent mesh per vertical section, so
// the deep underground can be skipped when it cannot be seen.
const chunkSections = 3
const sectionH = worldH / chunkSections

type chunk struct {
	opaque [chunkSections]*meshBuf
	trans  [chunkSections]*meshBuf
	dirty  bool
	ci, cj int
}

// Env is the per-frame lighting and fog state pushed to the world shader.
type Env struct {
	Light            float32
	Fog              rl.Color
	FogStart, FogEnd float32
	SunTint          [3]float32 // colour of sunlight (orange at dawn, blue at night)
	SunDir           rl.Vector3 // toward the sun, for directional face shading
	Flicker          float32    // torch light wobble around 1
	Time             float32
}

// World holds the voxel volume, per-column heights and chunk meshes.
type World struct {
	Blocks         []Block
	Height         []int        // per column (z*worldW+x): top of the column (highest non-air, including water) + 1
	Ground         []int        // per column: feet level on the highest solid block
	Light          []uint8      // per cell: sunlight in the high nibble, block light in the low nibble
	Version        int          // bumped on every block change; the nav grid watches it
	Seed           int          // generation seed; biomes are derived from it
	Beacon         rl.Vector3   // the objective's position
	VillageCentres []rl.Vector3 // squares of generated villages (world coordinates)
	BeaconLit      bool
	Biome          []Biome                    // per column
	OnSet          func(x, y, z int, b Block) // called after every Set (multiplayer broadcast)
	relight        map[int]*editBox           // chunks with pending edits and the cells they touched
	lightOld       []uint8                    // relight scratch: the region's previous light
	lastReady      []cand                     // chunks drawn by this frame's opaque pass
	scratchDirty   []cand                     // visibleChunks scratch
	lightQueue     []int32                    // relight scratch: the flood-fill frontier
	sunBottom      []int32                    // relight scratch: lowest sunlit cell per column
	xsScratch      []int32                    // relight scratch: wrapped x of each region column
	navDirty       []int32                    // columns edited since the nav grid last looked
	navAll         bool                       // too many edits to list: the nav grid refreshes fully
	chunks         []*chunk
	ncx            int
	ncz            int

	gpu       bool // GPU resources created
	tex       rl.Texture2D
	mat       rl.Material
	shader    rl.Shader // terrain
	eshader   rl.Shader // entities
	shaderOK  bool
	locView   int32
	locFog    int32
	locFogS   int32
	locFogE   int32
	locLight  int32
	locSun    int32
	locSunD   int32
	locFlick  int32
	locTile   int32
	locScrol  int32
	locWater  int32
	locCel    int32
	elocView  int32
	elocFog   int32
	elocFogS  int32
	elocFogE  int32
	blockM    map[Block]*meshBuf // unit cube meshes for item drops
	envTime   float32
	envFogEnd float32
	warmed    bool // chunks in view have been built once
}

func NewWorld() *World {
	w := newEmptyWorld()
	w.Seed = rand.Int()
	w.generate(w.Seed)
	w.finish()
	return w
}

// NewWorldFromBlocks rebuilds a world from a saved voxel volume and its seed.
func NewWorldFromBlocks(blocks []Block, seed int) *World {
	w := newEmptyWorld()
	copy(w.Blocks, blocks)
	w.Seed = seed
	w.finish()
	w.findBeacon()
	return w
}

func (w *World) finish() {
	if w.Biome == nil {
		w.Biome = make([]Biome, worldW*worldD)
		for z := 0; z < worldD; z++ {
			for x := 0; x < worldW; x++ {
				w.Biome[z*worldW+x] = biomeAt(x, z, w.Seed)
			}
		}
	}
	for z := 0; z < worldD; z++ {
		for x := 0; x < worldW; x++ {
			w.recomputeHeight(x, z)
		}
	}
	w.relightRegion(0, 0, worldW, worldD, 0, worldH)
}

func newEmptyWorld() *World {
	w := &World{
		Blocks:  make([]Block, worldW*worldH*worldD),
		Height:  make([]int, worldW*worldD),
		Ground:  make([]int, worldW*worldD),
		Light:   make([]uint8, worldW*worldH*worldD),
		relight: map[int]*editBox{},
		ncx:     worldW / chunkSize,
		ncz:     worldD / chunkSize,
		blockM:  map[Block]*meshBuf{},
	}
	for i := 0; i < w.ncx*w.ncz; i++ {
		c := &chunk{dirty: true, ci: i % w.ncx, cj: i / w.ncx}
		for k := 0; k < chunkSections; k++ {
			c.opaque[k], c.trans[k] = &meshBuf{wobble: true}, &meshBuf{wobble: true}
		}
		w.chunks = append(w.chunks, c)
	}
	return w
}

// ---------- lighting ----------

func cellIndex(x, y, z int) int { return (y*worldD+wrapZ(z))*worldW + wrapX(x) }

// sunLocal and blockLocal return the two light channels of a local cell (full sun outside the volume).
func (w *World) sunLocal(x, y, z int) int {
	if y >= worldH {
		return 15
	}
	if y < 0 {
		return 0
	}
	return int(w.Light[cellIndex(x, y, z)] >> 4)
}

func (w *World) blockLocal(x, y, z int) int {
	if !inLocal(x, y, z) {
		return 0
	}
	return int(w.Light[cellIndex(x, y, z)] & 15)
}

// Luminance returns the brightness factor at a world position for the given daylight.
func (w *World) Luminance(p rl.Vector3, daylight float32) float32 {
	lx, y, lz := floorI(p.X)-originX, floorI(p.Y), floorI(p.Z)-originZ
	sun := float32(w.sunLocal(lx, y, lz)) / 15 * daylight
	blk := float32(w.blockLocal(lx, y, lz)) / 15
	return brightness(max(sun, blk))
}

// brightness maps a light level in 0..1 to a colour multiplier; mirrors the shader.
func brightness(l float32) float32 { return 0.03 + 0.97*l*l }

// relightRegion recomputes both light channels for the local columns
// [x0,x1) x [z0,z1) over the full height, seeded by sky exposure, torches and
// the light already present just outside the region. Chunks whose light
// changed are marked for re-meshing.
func (w *World) relightRegion(x0, z0, x1, z1, y0, y1 int) {
	relightCalls++
	// Regions may run past the edges; columns wrap. A region wider than the
	// world is the whole world.
	if x1-x0 >= worldW {
		x0, x1 = 0, worldW
	}
	if z1-z0 >= worldD {
		z0, z1 = 0, worldD
	}
	if x0 >= x1 || z0 >= z1 || y0 >= y1 {
		return
	}
	// Sunlight below the region depends on whether the column above it is
	// open. Where a column's answer changed, the region must extend down to
	// where that column's sunlight stops (the first block below) plus the
	// fifteen cells light can spread from there; not to bedrock, which used
	// to make every leaf grown over open grass relight a full-height slab.
	if y0 > 0 {
		floor := y0
		for z := z0; z < z1; z++ {
			for x := x0; x < x1; x++ {
				open := true
				for y := worldH - 1; y >= y0-1; y-- {
					if b := w.getLocal(wrapX(x), y, wrapZ(z)); b != Air && !blocks[b].Tiny {
						open = false
						break
					}
				}
				if was := w.Light[cellIndex(x, y0-1, z)]>>4 == 15; was == open {
					continue
				}
				stop := y0 - 1
				for stop > 0 {
					if b := w.getLocal(wrapX(x), stop-1, wrapZ(z)); b != Air && !blocks[b].Tiny {
						break
					}
					stop--
				}
				floor = min(floor, stop-16)
			}
		}
		y0 = max(0, floor)
	}
	relightCells += (x1 - x0) * (z1 - z0) * (y1 - y0)
	inRegion := func(x, y, z int) bool {
		return y >= y0 && y < y1 && wrapI(x-x0, worldW) < x1-x0 && wrapI(z-z0, worldD) < z1-z0
	}
	// Wrapped column coordinates, computed once rather than per cell.
	xs := w.xsScratch[:0]
	for x := x0; x < x1; x++ {
		xs = append(xs, int32(wrapX(x)))
	}
	w.xsScratch = xs
	old := w.lightOld[:0]
	for y := y0; y < y1; y++ {
		for z := z0; z < z1; z++ {
			row := (y*worldD + wrapZ(z)) * worldW
			for _, x := range xs {
				i := row + int(x)
				old = append(old, w.Light[i])
				w.Light[i] = 0
			}
		}
	}
	queue := w.lightQueue[:0]
	// Sunlight falls straight down through air until it hits anything.
	// sb records each column's lowest sunlit cell (y1 when none in range).
	rw, rd := x1-x0, z1-z0
	sb := w.sunBottom[:0]
	for z := z0; z < z1; z++ {
		wz := wrapZ(z)
		for _, wx := range xs {
			bottom := y1
			// Everything from the column's top (w.Height) up is air: sunlit
			// without looking at the blocks.
			top := w.Height[wz*worldW+int(wx)]
			for y := y1 - 1; y >= max(top, y0); y-- {
				w.Light[(y*worldD+wz)*worldW+int(wx)] = 15 << 4
				bottom = y
			}
			for y := min(top, worldH) - 1; y >= y0; y-- {
				b := w.Blocks[(y*worldD+wz)*worldW+int(wx)]
				if b != Air && !blocks[b].Tiny {
					break
				}
				if y < y1 {
					w.Light[(y*worldD+wz)*worldW+int(wx)] = 15 << 4
					bottom = y
				}
			}
			sb = append(sb, int32(bottom))
		}
	}
	w.sunBottom = sb
	// Only sunlit cells that border an unlit cell inside the region join the
	// flood fill: the lowest one in each column, and those beside a
	// neighbouring column whose sunlight stops higher up. The open sky is most
	// of a region's cells and the fill had nothing to do there.
	for z := z0; z < z1; z++ {
		for x := x0; x < x1; x++ {
			bottom := int(sb[(z-z0)*rw+x-x0])
			if bottom >= y1 {
				continue
			}
			edge := bottom + 1
			for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				nx, nz := x+d[0], z+d[1]
				// A region the full width of the world wraps onto itself.
				if rw == worldW {
					nx = x0 + wrapI(nx-x0, worldW)
				}
				if rd == worldD {
					nz = z0 + wrapI(nz-z0, worldD)
				}
				if nx >= x0 && nx < x1 && nz >= z0 && nz < z1 {
					edge = max(edge, int(sb[(nz-z0)*rw+nx-x0]))
				}
			}
			for y := bottom; y < min(edge, y1); y++ {
				queue = append(queue, int32(cellIndex(x, y, z)))
			}
		}
	}
	// Light flowing in from the surrounding cells: the columns around the
	// region and the layers just above and below it.
	seed := func(x, y, z int) {
		if y < 0 || y >= worldH || inRegion(x, y, z) {
			return
		}
		if w.Light[cellIndex(x, y, z)] != 0 {
			queue = append(queue, int32(cellIndex(x, y, z)))
		}
	}
	if x1-x0 < worldW || z1-z0 < worldD {
		for y := y0 - 1; y <= y1; y++ {
			for x := x0 - 1; x <= x1; x++ {
				seed(x, y, z0-1)
				seed(x, y, z1)
			}
			for z := z0; z < z1; z++ {
				seed(x0-1, y, z)
				seed(x1, y, z)
			}
		}
	}
	for z := z0; z < z1; z++ {
		for x := x0; x < x1; x++ {
			seed(x, y0-1, z)
			seed(x, y1, z)
		}
	}
	// Torches.
	for y := y0; y < y1; y++ {
		for z := z0; z < z1; z++ {
			for x := x0; x < x1; x++ {
				if e := blocks[w.getLocal(wrapX(x), y, wrapZ(z))].Emit; e > 0 {
					i := cellIndex(x, y, z)
					w.Light[i] = w.Light[i]&0xf0 | e
					queue = append(queue, int32(i))
				}
			}
		}
	}
	// Breadth-first spread of both channels; cells outside the region are read but never written.
	dirs := [6][3]int{{1, 0, 0}, {-1, 0, 0}, {0, 1, 0}, {0, -1, 0}, {0, 0, 1}, {0, 0, -1}}
	for head := 0; head < len(queue); head++ {
		i := int(queue[head])
		x := i % worldW
		z := (i / worldW) % worldD
		y := i / (worldW * worldD)
		v := w.Light[i]
		sun, blk := int(v>>4), int(v&15)
		if sun <= 1 && blk <= 1 {
			continue
		}
		for _, d := range dirs {
			nx, ny, nz := wrapX(x+d[0]), y+d[1], wrapZ(z+d[2])
			if !inRegion(nx, ny, nz) {
				continue
			}
			cost := int(w.getLocal(nx, ny, nz).lightCost())
			if cost == 0 {
				continue
			}
			ni := cellIndex(nx, ny, nz)
			nv := w.Light[ni]
			ns, nb := max(int(nv>>4), sun-cost), max(int(nv&15), blk-cost)
			if ns != int(nv>>4) || nb != int(nv&15) {
				w.Light[ni] = uint8(ns<<4 | nb)
				queue = append(queue, int32(ni))
			}
		}
	}
	// Re-mesh every chunk whose light changed.
	k := 0
	for y := y0; y < y1; y++ {
		for z := z0; z < z1; z++ {
			wz := wrapZ(z)
			row := (y*worldD + wz) * worldW
			for _, x := range xs {
				if old[k] != w.Light[row+int(x)] {
					w.chunks[(wz/chunkSize)*w.ncx+int(x)/chunkSize].dirty = true
				}
				k++
			}
		}
	}
	// Keep the scratch space for the next relight, but not the whole-world
	// sized arrays the first light pass needed.
	if cap(old) <= 1<<22 {
		w.lightOld = old
	}
	if cap(queue) <= 1<<22 {
		w.lightQueue = queue
	}
}

// editBox is the span of cells edited in one chunk since the last relight (local coordinates).
type editBox struct{ x0, y0, z0, x1, y1, z1 int }

// cost is the number of cells a relight of this box visits once it has been
// grown by the distance light travels.
func (b editBox) cost() int {
	const reach = 16
	return (b.x1 - b.x0 + 2*reach) * (b.z1 - b.z0 + 2*reach) * (b.y1 - b.y0 + 2*reach)
}

// lightAlike reports whether swapping one block for the other leaves every
// light calculation the same: same cost to pass through, same emission and
// the same effect on a column of sunlight.
func lightAlike(a, b Block) bool {
	sunA, sunB := a == Air || blocks[a].Tiny, b == Air || blocks[b].Tiny
	return a.lightCost() == b.lightCost() && blocks[a].Emit == blocks[b].Emit && sunA == sunB
}

var relightCalls, relightCells int // profiling counters for the PERF log

// flushLight recomputes lighting around cells edited since the last frame.
// Edits in nearby chunks are merged into one region; each region is the
// edits' bounding box grown by the 15 blocks light can travel, in height as
// well, so a leaf growing on one tree does not relight three chunks of sky.
// Regions are processed within a time budget and the rest wait for the next
// frame: a growth tick that touches trees all over the map used to relight
// them all at once and freeze a frame for most of a tenth of a second.
func (w *World) flushLight() {
	if len(w.relight) == 0 {
		return
	}
	type region struct {
		key int
		box editBox
	}
	var regions []region
	done := map[int]bool{}
	for ci, box := range w.relight {
		if done[ci] {
			continue
		}
		i, j := ci%w.ncx, ci/w.ncx
		u := *box
		for cj, bj := range w.relight {
			if done[cj] {
				continue
			}
			ii, jj := cj%w.ncx, cj/w.ncx
			if abs(ii-i) > 2 || abs(jj-j) > 2 {
				continue
			}
			// Merge only when one relight of the union is cheaper than two:
			// leaves growing on trees three chunks apart used to be swept
			// into a single slab mostly made of untouched ground and sky.
			m := editBox{min(u.x0, bj.x0), min(u.y0, bj.y0), min(u.z0, bj.z0), max(u.x1, bj.x1), max(u.y1, bj.y1), max(u.z1, bj.z1)}
			if m.cost() <= u.cost()+bj.cost() {
				u = m
				done[cj] = true
			}
		}
		done[ci] = true
		regions = append(regions, region{ci, u})
	}
	w.relight = map[int]*editBox{}
	const reach = 16
	const budget = 3 * time.Millisecond
	start := time.Now()
	for n, r := range regions {
		if n > 0 && time.Since(start) > budget {
			// Back in the queue; later edits nearby merge into it as usual.
			box := r.box
			w.relight[r.key] = &box
			continue
		}
		u := r.box
		t0, c0 := time.Now(), relightCells
		w.relightRegion(u.x0-reach, u.z0-reach, u.x1+reach, u.z1+reach, max(0, u.y0-reach), min(worldH, u.y1+reach))
		if perfTrace {
			rl.TraceLog(rl.LogInfo, "RELIGHT %dx%dx%d cells=%dk %.1fms", u.x1-u.x0+2*reach, u.z1-u.z0+2*reach, u.y1-u.y0+2*reach, (relightCells-c0)/1000, ms(time.Since(t0)))
		}
	}
}

// ---------- generation ----------

func hash2(x, y, seed int) float32 {
	h := uint32(x)*374761393 + uint32(y)*668265263 + uint32(seed)*1274126177
	h = (h ^ (h >> 13)) * 1274126177
	h ^= h >> 16
	return float32(h&0xffff) / 65535
}

func smooth(f float32) float32 { return f * f * (3 - 2*f) }

// vnoise is smooth 2D value noise in [0,1].
func vnoise(x, y float32, seed int) float32 {
	xi, yi := floorI(x), floorI(y)
	fx, fy := smooth(x-float32(xi)), smooth(y-float32(yi))
	a, b := hash2(xi, yi, seed), hash2(xi+1, yi, seed)
	c, d := hash2(xi, yi+1, seed), hash2(xi+1, yi+1, seed)
	return lerp(lerp(a, b, fx), lerp(c, d, fx), fy)
}

func wrapI(v, period int) int { return ((v % period) + period) % period }

// pnoise is vnoise whose lattice repeats every px by py cells, so terrain built
// from it tiles seamlessly across the world's wrapped edges.
func pnoise(x, y float32, seed, px, py int) float32 {
	xi, yi := floorI(x), floorI(y)
	fx, fy := smooth(x-float32(xi)), smooth(y-float32(yi))
	h := func(dx, dy int) float32 { return hash2(wrapI(xi+dx, px), wrapI(yi+dy, py), seed) }
	return lerp(lerp(h(0, 0), h(1, 0), fx), lerp(h(0, 1), h(1, 1), fx), fy)
}

// vnoise3 is smooth 3D value noise in [0,1], periodic in x and z every px/pz
// lattice cells (0 means not periodic), used to carve caves.
func vnoise3(x, y, z float32, seed int) float32 { return vnoise3p(x, y, z, seed, 0, 0) }

func vnoise3p(x, y, z float32, seed, px, pz int) float32 {
	xi, yi, zi := floorI(x), floorI(y), floorI(z)
	fx, fy, fz := smooth(x-float32(xi)), smooth(y-float32(yi)), smooth(z-float32(zi))
	h := func(dx, dy, dz int) float32 {
		ax, az := xi+dx, zi+dz
		if px > 0 {
			ax, az = wrapI(ax, px), wrapI(az, pz)
		}
		return hash2(ax, (yi+dy)*1013+az, seed)
	}
	c00 := lerp(h(0, 0, 0), h(1, 0, 0), fx)
	c10 := lerp(h(0, 1, 0), h(1, 1, 0), fx)
	c01 := lerp(h(0, 0, 1), h(1, 0, 1), fx)
	c11 := lerp(h(0, 1, 1), h(1, 1, 1), fx)
	return lerp(lerp(c00, c10, fy), lerp(c01, c11, fy), fz)
}

// biomeAt picks the biome of a local column from a slow noise field.
func biomeAt(x, z int, seed int) Biome {
	b := pnoise(float32(x)/48, float32(z)/48, seed+20, worldW/48, worldD/48)
	switch {
	case b < 0.22:
		return BiomeDesert
	case b < 0.36:
		return BiomeOutback
	case b < 0.5:
		return BiomePlains
	case b < 0.6:
		return BiomeSwamp
	case b < 0.78:
		return BiomeForest
	}
	return BiomeTaiga
}

func (w *World) generate(seed int) {
	heights := make([]int, worldW*worldD)
	biomes := make([]Biome, worldW*worldD)
	w.Biome = biomes
	// Terrain: rolling hills, a mountain band, beaches and a sea.
	for z := 0; z < worldD; z++ {
		for x := 0; x < worldW; x++ {
			fx, fz := float32(x), float32(z)
			n := 0.5*pnoise(fx/48, fz/48, seed, worldW/48, worldD/48) + 0.3*pnoise(fx/16, fz/16, seed+1, worldW/16, worldD/16) +
				0.15*pnoise(fx/6, fz/6, seed+2, worldW/6, worldD/6) + 0.05*pnoise(fx/3, fz/3, seed+3, worldW/3, worldD/3)
			h := groundBase + 5 + int(n*n*30+n*6)
			if m := pnoise(fx/32, fz/32, seed+4, worldW/32, worldD/32); m > 0.62 {
				h += int((m - 0.62) * 70)
			}
			// Flatten the middle so the player spawn is open and dry.
			dx, dz := float32(x+originX), float32(z+originZ)
			if d := float32(math.Sqrt(float64(dx*dx + dz*dz))); d < 10 {
				t := clamp(d/10, 0, 1)
				h = int(lerp(groundBase+14, float32(h), t*t))
			}
			h = int(clamp(float32(h), groundBase+5, worldH-8))
			biome := biomeAt(x, z, seed)
			biomes[z*worldW+x] = biome
			if biome == BiomeSwamp {
				// Flatten toward just above the sea, with scattered pools cut a block below it.
				h = int(lerp(float32(h), seaLevel+2, 0.8))
				if pnoise(fx/9, fz/9, seed+21, worldW/9, worldD/9) > 0.6 {
					h = seaLevel
				}
			}
			heights[z*worldW+x] = h
			sandy := (h <= seaLevel+2 && biome != BiomeSwamp) || biome == BiomeDesert
			outback := biome == BiomeOutback && h > seaLevel+2
			swamp := biome == BiomeSwamp
			for y := 0; y < h; y++ {
				b := Stone
				switch {
				case y == 0 || (y == 1 && hash2(x, z, seed+7) < 0.4):
					b = Bedrock
				case y == h-1:
					b = Grass
					if swamp && (h <= seaLevel || hash2(x, z, seed+22) < 0.35) {
						b = Mud
					} else if sandy {
						b = Sand
					} else if outback {
						b = RedSand
					} else if h >= groundBase+40 || (biome == BiomeTaiga && h >= groundBase+20) {
						b = Snow
					}
				case y >= h-4:
					b = Dirt
					if swamp && y >= h-2 {
						b = Mud
					} else if sandy {
						b = Sand
					} else if outback && y >= h-3 {
						b = RedSand
					}
				default:
					if y < deepTop {
						b = DeepStone
					} else if vnoise3p(fx/8, float32(y)/7, fz/8, seed+5, worldW/8, worldD/8) > 0.8 {
						b = Gravel
					}
				}
				w.setLocal(x, y, z, b)
			}
			for y := h; y <= seaLevel; y++ {
				w.setLocal(x, y, z, Water)
			}
		}
	}
	// Caves: winding tunnels where two noise fields cross, plus a few caverns.
	for z := 0; z < worldD; z++ {
		for x := 0; x < worldW; x++ {
			h := heights[z*worldW+x]
			top := h - 1
			if h <= seaLevel+1 {
				top = h - 4 // keep the sea floor watertight
			}
			for y := 2; y < top; y++ {
				fx, fy, fz := float32(x), float32(y), float32(z)
				a := vnoise3p(fx/12, fy/9, fz/12, seed+11, worldW/12, worldD/12)
				b := vnoise3p(fx/12, fy/9, fz/12, seed+12, worldW/12, worldD/12)
				width := float32(0.05)
				cavernAt := float32(0.76)
				if y < deepTop {
					width = 0.075                                                          // wider tunnels in the deep
					cavernAt = 0.68 - 0.1*clamp(float32(deepTop-y)/float32(deepTop), 0, 1) // and vast caverns
				}
				tunnel := math.Abs(float64(a-0.5)) < float64(width) && math.Abs(float64(b-0.5)) < float64(width)
				cavern := vnoise3p(fx/8, fy/6, fz/8, seed+13, worldW/8, worldD/8) > cavernAt
				if y < deepTop && vnoise3p(fx/16, fy/10, fz/16, seed+14, worldW/16, worldD/16) > 0.72 {
					cavern = true // deep halls
				}
				if tunnel || cavern {
					w.setLocal(x, y, z, Air)
				}
			}
		}
	}
	// Lava pools at the very bottom, underground lakes a little higher, and
	// glowing mushrooms and amethyst in the deep.
	for z := 0; z < worldD; z++ {
		for x := 0; x < worldW; x++ {
			for y := 1; y <= 8; y++ {
				if w.getLocal(x, y, z) == Air && heights[z*worldW+x] > y+3 {
					w.setLocal(x, y, z, Lava)
				}
			}
			for y := 9; y <= 16; y++ {
				if w.getLocal(x, y, z) == Air && heights[z*worldW+x] > y+3 && pnoise(float32(x)/24, float32(z)/24, seed+15, worldW/24, worldD/24) > 0.55 {
					w.setLocal(x, y, z, Water)
				}
			}
			for y := 2; y < deepTop; y++ {
				if w.getLocal(x, y, z) == Air && blocks[w.getLocal(x, y-1, z)].Solid && w.getLocal(x, y-1, z) != Lava {
					r := hash2(x, y*77+z, seed+16)
					if r < 0.03 {
						w.setLocal(x, y, z, Glowshroom)
					}
				}
			}
		}
	}
	// Ravines: long narrow chasms from the surface down into the deep.
	for i := 0; i < 2*areaScale; i++ {
		x, z := rand.Intn(worldW), rand.Intn(worldD)
		if dx, dz := x+originX, z+originZ; dx*dx+dz*dz < 900 {
			continue
		}
		ang := rand.Float64() * 2 * math.Pi
		dx, dz := math.Cos(ang), math.Sin(ang)
		length := 30 + rand.Intn(40)
		floorY := deepTop - 6 - rand.Intn(10)
		for k := 0; k < length; k++ {
			cx, cz := x+int(float64(k)*dx), z+int(float64(k)*dz)
			half := 1 + int(2*math.Sin(float64(k)/float64(length)*math.Pi)) // widest in the middle
			top := heights[wrapZ(cz)*worldW+wrapX(cx)]
			if top <= seaLevel+1 {
				continue
			}
			for s := -half; s <= half; s++ {
				ox, oz := cx+int(float64(s)*-dz), cz+int(float64(s)*dx)
				for y := floorY; y < top+2; y++ {
					if b := w.getLocal(ox, y, oz); b != Bedrock && b != Lava && b != Water {
						w.setLocal(ox, y, oz, Air)
					}
				}
			}
			ang += (rand.Float64() - 0.5) * 0.15
			dx, dz = math.Cos(ang), math.Sin(ang)
		}
	}
	// Ore veins, deeper ores rarer.
	vein := func(ore Block, count, minY, maxY, size int) {
		for i := 0; i < count; i++ {
			x, z := rand.Intn(worldW), rand.Intn(worldD)
			y := minY + rand.Intn(maxY-minY+1)
			for j := 0; j < size; j++ {
				if b := w.getLocal(x, y, z); b == Stone || b == DeepStone {
					w.setLocal(x, y, z, ore)
				}
				x += rand.Intn(3) - 1
				y += rand.Intn(3) - 1
				z += rand.Intn(3) - 1
			}
		}
	}
	vein(CoalOre, 300*areaScale, 4, groundBase+42, 9)
	vein(IronOre, 190*areaScale, 2, groundBase+28, 6)
	vein(GoldOre, 70*areaScale, 2, deepTop+8, 5)
	vein(DiamondOre, 40*areaScale, 1, deepTop-6, 4)
	vein(Amethyst, 60*areaScale, 2, deepTop-4, 5)
	// Trees, cacti and ground cover by biome.
	for i := 0; i < 900*areaScale; i++ {
		x, z := rand.Intn(worldW-4)+2, rand.Intn(worldD-4)+2
		if dx, dz := x+originX, z+originZ; dx*dx+dz*dz < 49 {
			continue
		}
		h := heights[z*worldW+x]
		top := w.getLocal(x, h-1, z)
		if w.getLocal(x, h, z) != Air {
			continue
		}
		r := rand.Float32()
		switch biomes[z*worldW+x] {
		case BiomeDesert:
			if top == Sand && h >= seaLevel && h <= seaLevel+3 && r < 0.05 {
				w.placePalm(x, h, z, w.setLocal)
			} else if top == Sand && h > seaLevel+1 && r < 0.12 {
				for y := h; y < h+1+rand.Intn(3); y++ {
					w.setLocal(x, y, z, Cactus)
				}
			}
		case BiomePlains:
			switch {
			case top == Grass && r < 0.025:
				w.placeTree(x, h, z, w.setLocal)
			case top == Grass && r < 0.045:
				w.placeAcacia(x, h, z, w.setLocal)
			case top == Grass && r < 0.06:
				// Wild vegetables: a carrot or beetroot patch, the odd pumpkin.
				switch rand.Intn(3) {
				case 0:
					w.setLocal(x, h, z, CarrotRipe)
				case 1:
					w.setLocal(x, h, z, BeetrootRipe)
				default:
					w.setLocal(x, h, z, Pumpkin)
				}
			case top == Grass && r < 0.45:
				w.setLocal(x, h, z, TallGrass)
			case top == Grass && r < 0.55:
				w.setLocal(x, h, z, Flower)
			}
		case BiomeForest:
			switch {
			case top == Grass && r < 0.3:
				w.placeTree(x, h, z, w.setLocal)
			case top == Grass && r < 0.42:
				w.placeBirch(x, h, z, w.setLocal)
			case top == Grass && r < 0.47:
				w.placeCherry(x, h, z, w.setLocal)
			case top == Grass && r < 0.48:
				w.setLocal(x, h, z, Pumpkin)
			case top == Grass && r < 0.6:
				w.setLocal(x, h, z, TallGrass)
			}
		case BiomeTaiga:
			if (top == Grass || top == Snow) && r < 0.3 {
				w.placeSpruce(x, h, z, w.setLocal)
			}
		case BiomeSwamp:
			switch {
			case (top == Grass || top == Mud) && h > seaLevel && r < 0.09:
				w.placeWillow(x, h, z, w.setLocal)
			case top == Grass && h > seaLevel && r < 0.11:
				w.setLocal(x, h, z, Melon) // wild watermelons
			case (top == Grass || top == Mud) && r < 0.5:
				w.setLocal(x, h, z, TallGrass)
			case top == Mud && h <= seaLevel+1 && r < 0.6:
				w.setLocal(x, h, z, Glowshroom) // marsh lights
			}
		case BiomeOutback:
			switch {
			case top == RedSand && r < 0.035:
				w.placeEucalyptus(x, h, z, w.setLocal)
			case top == RedSand && r < 0.14:
				w.setLocal(x, h, z, DeadBush)
			}
		}
	}
	// Dungeons: dark cobblestone rooms deep underground with a spawner and loot.
	for i := 0; i < 7*areaScale; i++ {
		x, z := rand.Intn(worldW-12)+6, rand.Intn(worldD-12)+6
		if dx, dz := x+originX, z+originZ; dx*dx+dz*dz < 144 {
			continue
		}
		y := 5 + rand.Intn(groundBase+12)
		if heights[z*worldW+x] < y+8 {
			continue
		}
		for dz := -3; dz <= 3; dz++ {
			for dx := -3; dx <= 3; dx++ {
				for dy := -1; dy <= 4; dy++ {
					wall := abs(dx) == 3 || abs(dz) == 3 || dy == -1 || dy == 4
					b := Air
					if wall {
						b = Cobble
						if hash2(x+dx, (y+dy)*131+z+dz, seed+15) < 0.35 {
							b = StoneBrick
						}
					}
					w.setLocal(x+dx, y+dy, z+dz, b)
				}
			}
		}
		w.setLocal(x, y, z, Spawner)
		w.setLocal(x-2, y, z-2, Crate)
		if rand.Intn(2) == 0 {
			w.setLocal(x+2, y, z+2, Crate)
		}
		// One doorway so cave explorers can find it.
		w.setLocal(x+3, y, z, Air)
		w.setLocal(x+3, y+1, z, Air)
	}
	// Villages on flat grassland, and the beacon tower far from spawn.
	w.placeVillages(heights)
	w.placeBeaconTower(heights)
	// Abandoned mineshafts: long timbered corridors with a few crates.
	for i := 0; i < 4*areaScale; i++ {
		x, z := rand.Intn(worldW-40)+20, rand.Intn(worldD-40)+20
		y := 8 + rand.Intn(groundBase+10)
		if heights[z*worldW+x]-12 < y {
			y = heights[z*worldW+x] - 12 // stay well under the surface
		}
		if y < 4 {
			continue
		}
		dx, dz := 1, 0
		if rand.Intn(2) == 0 {
			dx, dz = 0, 1
		}
		length := 20 + rand.Intn(20)
		for k := 0; k < length; k++ {
			cx, cz := x+dx*k, z+dz*k
			if cx < 2 || cz < 2 || cx >= worldW-2 || cz >= worldD-2 {
				break
			}
			for s := -1; s <= 1; s++ {
				for dy := 0; dy < 3; dy++ {
					ox, oz := cx+s*dz, cz+s*dx
					w.setLocal(ox, y+dy, oz, Air)
				}
				w.setLocal(cx+s*dz, y-1, cz+s*dx, Planks) // plank floor
			}
			if k%4 == 0 {
				// Timber frame: two posts and a beam.
				for dy := 0; dy < 2; dy++ {
					w.setLocal(cx-dz, y+dy, cz-dx, Log)
					w.setLocal(cx+dz, y+dy, cz+dx, Log)
				}
				for s := -1; s <= 1; s++ {
					w.setLocal(cx+s*dz, y+2, cz+s*dx, Planks)
				}
				if rand.Intn(3) == 0 {
					w.setLocal(cx, y+1, cz, Torch)
				}
			}
			if rand.Intn(14) == 0 {
				w.setLocal(cx+dz, y, cz+dx, Crate)
			}
		}
	}
	// Stone-brick ruins for cover.
	ruins := [][2]int{{-22, -22}, {22, -22}, {-22, 22}, {22, 22}, {0, -30}, {0, 30}, {-32, 0}, {32, 0}, {-14, 34}, {36, -14}}
	for i := 0; i < 10*(areaScale-1); i++ {
		ruins = append(ruins, [2]int{rand.Intn(worldW-12) + 6 + originX, rand.Intn(worldD-12) + 6 + originZ})
	}
	for _, c := range ruins {
		x, z := c[0]-originX, c[1]-originZ
		if x < 3 || z < 3 || x >= worldW-3 || z >= worldD-3 {
			continue
		}
		h := heights[z*worldW+x]
		if h <= seaLevel+1 {
			continue
		}
		for dz := -2; dz <= 2; dz++ {
			for dx := -2; dx <= 2; dx++ {
				for y := 0; y < h; y++ {
					if w.getLocal(x+dx, y, z+dz) == Air || w.getLocal(x+dx, y, z+dz) == Water {
						w.setLocal(x+dx, y, z+dz, Stone)
					}
				}
				w.setLocal(x+dx, h-1, z+dz, Cobble)
				for y := h; y < h+4; y++ {
					w.setLocal(x+dx, y, z+dz, Air)
				}
				edge := abs(dx) == 2 || abs(dz) == 2
				door := (dx == 0 && dz == 2) || (dz == 0 && dx == -2)
				if edge && !door {
					for y := h; y < h+3; y++ {
						if y == h+2 && hash2(x+dx, z+dz, seed+9) < 0.3 {
							continue // crumbled top
						}
						w.setLocal(x+dx, y, z+dz, StoneBrick)
					}
				}
			}
		}
	}
}

// placeTree writes an oak with its trunk base at local (x, y, z) through set.
func (w *World) placeTree(x, y, z int, set func(x, y, z int, b Block)) {
	th := 4 + rand.Intn(3)
	for yy := y; yy < y+th; yy++ {
		set(x, yy, z, Log)
	}
	top := y + th - 1
	for dy := -2; dy <= 1; dy++ {
		r := 2
		if dy == 1 {
			r = 1
		}
		for dz := -r; dz <= r; dz++ {
			for dx := -r; dx <= r; dx++ {
				if abs(dx) == 2 && abs(dz) == 2 && (dy == -2 || rand.Intn(2) == 0) {
					continue
				}
				if b := w.getLocal(x+dx, top+dy, z+dz); b == Air || b == Sapling || b == TallGrass {
					set(x+dx, top+dy, z+dz, Leaves)
				}
			}
		}
	}
}

// placeBirch writes a birch: a taller pale trunk with an oak-style canopy.
func (w *World) placeBirch(x, y, z int, set func(x, y, z int, b Block)) {
	th := 5 + rand.Intn(3)
	for yy := y; yy < y+th; yy++ {
		set(x, yy, z, BirchLog)
	}
	top := y + th - 1
	for dy := -2; dy <= 1; dy++ {
		r := 2
		if dy == 1 {
			r = 1
		}
		for dz := -r; dz <= r; dz++ {
			for dx := -r; dx <= r; dx++ {
				if abs(dx) == 2 && abs(dz) == 2 {
					continue
				}
				if b := w.getLocal(x+dx, top+dy, z+dz); b == Air || b == TallGrass {
					set(x+dx, top+dy, z+dz, Leaves)
				}
			}
		}
	}
}

// placeEucalyptus writes a tall pale trunk with a sparse, high canopy.
func (w *World) placeEucalyptus(x, y, z int, set func(x, y, z int, b Block)) {
	th := 7 + rand.Intn(4)
	for yy := y; yy < y+th; yy++ {
		set(x, yy, z, EucLog)
	}
	top := y + th - 1
	for i := 0; i < 3; i++ {
		cx, cz, cy := x+rand.Intn(3)-1, z+rand.Intn(3)-1, top-rand.Intn(3)
		for dy := -1; dy <= 1; dy++ {
			for dz := -1; dz <= 1; dz++ {
				for dx := -1; dx <= 1; dx++ {
					if abs(dx)+abs(dz)+abs(dy) > 2 {
						continue
					}
					if w.getLocal(cx+dx, cy+dy, cz+dz) == Air {
						set(cx+dx, cy+dy, cz+dz, EucLeaves)
					}
				}
			}
		}
	}
}

// placeWillow writes a squat trunk with a wide, drooping canopy that hangs
// down around it: plenty of leaves for a brontosaur to reach.
func (w *World) placeWillow(x, y, z int, set func(x, y, z int, b Block)) {
	th := 4 + rand.Intn(2)
	for yy := y; yy < y+th; yy++ {
		set(x, yy, z, Log)
	}
	top := y + th - 1
	for dz := -3; dz <= 3; dz++ {
		for dx := -3; dx <= 3; dx++ {
			d := abs(dx) + abs(dz)
			if d > 4 {
				continue
			}
			for dy := 0; dy <= 1; dy++ {
				if w.getLocal(x+dx, top+dy, z+dz) == Air {
					set(x+dx, top+dy, z+dz, Leaves)
				}
			}
			// Hanging strands on the rim.
			if d >= 3 {
				for dy := -1; dy >= -2-rand.Intn(2); dy-- {
					if w.getLocal(x+dx, top+dy, z+dz) == Air {
						set(x+dx, top+dy, z+dz, Leaves)
					}
				}
			}
		}
	}
}

// placeSpruce writes a tall conical spruce.
func (w *World) placeSpruce(x, y, z int, set func(x, y, z int, b Block)) {
	th := 7 + rand.Intn(3)
	for yy := y; yy < y+th; yy++ {
		set(x, yy, z, Log)
	}
	for dy := 2; dy <= th; dy++ {
		r := 0
		switch {
		case dy == th:
			r = 0
		case dy >= th-2:
			r = 1
		case dy%2 == 0:
			r = 2
		default:
			r = 1
		}
		for dz := -r; dz <= r; dz++ {
			for dx := -r; dx <= r; dx++ {
				if abs(dx) == r && abs(dz) == r && r == 2 {
					continue
				}
				if b := w.getLocal(x+dx, y+dy, z+dz); b == Air {
					set(x+dx, y+dy, z+dz, SpruceLeaves)
				}
			}
		}
	}
	set(x, y+th, z, SpruceLeaves)
}

// placeAcacia writes a savanna tree: a short trunk that leans, forks, and
// opens into a flat, wide crown.
func (w *World) placeAcacia(x, y, z int, set func(x, y, z int, b Block)) {
	th := 4 + rand.Intn(2)
	lean := [2]int{rand.Intn(3) - 1, rand.Intn(3) - 1}
	cx, cz := x, z
	for yy := y; yy < y+th; yy++ {
		if yy >= y+2 {
			cx, cz = x+lean[0], z+lean[1]
		}
		set(cx, yy, z, AcaciaLog)
		if cz != z {
			set(cx, yy, cz, AcaciaLog)
		}
	}
	top := y + th - 1
	for dz := -3; dz <= 3; dz++ {
		for dx := -3; dx <= 3; dx++ {
			d := dx*dx + dz*dz
			if d > 10 {
				continue
			}
			if b := w.getLocal(cx+dx, top, cz+dz); b == Air || b == TallGrass {
				set(cx+dx, top, cz+dz, AcaciaLeaves)
			}
			if d <= 2 && w.getLocal(cx+dx, top+1, cz+dz) == Air {
				set(cx+dx, top+1, cz+dz, AcaciaLeaves)
			}
		}
	}
}

// placePalm writes a beach palm: a tall trunk that curves over, a tuft of
// fronds drooping from the top, and a coconut or two under them.
func (w *World) placePalm(x, y, z int, set func(x, y, z int, b Block)) {
	th := 6 + rand.Intn(3)
	dir := [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}[rand.Intn(4)]
	cx, cz := x, z
	for yy := y; yy < y+th; yy++ {
		if yy == y+th/2 || yy == y+th-2 {
			cx, cz = cx+dir[0], cz+dir[1]
		}
		set(cx, yy, cz, PalmLog)
	}
	top := y + th - 1
	set(cx, top+1, cz, PalmLeaves)
	// Eight fronds arch out from the crown and droop at the tips: long ones
	// along the axes, shorter ones on the diagonals.
	for _, d := range [8][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {-1, 1}, {1, -1}, {-1, -1}} {
		rise := []int{1, 1, 0, -1} // height of each step out from the trunk
		if d[0] != 0 && d[1] != 0 {
			rise = []int{1, 0}
		}
		for i, dy := range rise {
			fx, fz := cx+d[0]*(i+1), cz+d[1]*(i+1)
			if w.getLocal(fx, top+dy, fz) == Air {
				set(fx, top+dy, fz, PalmLeaves)
			}
		}
	}
}

// placeCherry writes a cherry tree: a dark trunk with a round pink crown.
func (w *World) placeCherry(x, y, z int, set func(x, y, z int, b Block)) {
	th := 4 + rand.Intn(2)
	for yy := y; yy < y+th; yy++ {
		set(x, yy, z, CherryLog)
	}
	top := y + th - 1
	for dy := -1; dy <= 2; dy++ {
		r := 3
		if dy == 2 || dy == -1 {
			r = 2
		}
		for dz := -r; dz <= r; dz++ {
			for dx := -r; dx <= r; dx++ {
				if dx*dx+dz*dz > r*r+1 {
					continue
				}
				if b := w.getLocal(x+dx, top+dy, z+dz); b == Air || b == TallGrass || b == Flower {
					set(x+dx, top+dy, z+dz, CherryLeaves)
				}
			}
		}
	}
}

// isLog and leafFor describe trunks and the leaves they regrow.
func isLog(b Block) bool {
	return b == Log || b == BirchLog || b == EucLog || b == AcaciaLog || b == PalmLog || b == CherryLog
}

func (w *World) leafFor(lx, y, lz int, log Block) Block {
	switch log {
	case EucLog:
		return EucLeaves
	case AcaciaLog:
		return AcaciaLeaves
	case PalmLog:
		return PalmLeaves
	case CherryLog:
		return CherryLeaves
	}
	// Spruce and oak share the log: copy whatever leaves the tree already has.
	for _, d := range [][3]int{{1, 0, 0}, {-1, 0, 0}, {0, 0, 1}, {0, 0, -1}, {0, 1, 0}, {1, 1, 0}, {-1, 1, 0}, {0, 1, 1}, {0, 1, -1}} {
		if b := w.getLocal(lx+d[0], y+d[1], lz+d[2]); b == SpruceLeaves {
			return SpruceLeaves
		}
	}
	return Leaves
}

// RegrowTrees gives damaged trees their leaves back and lets oaks bear fruit.
// It samples a slice of the world each call so the cost stays small.
func (w *World) RegrowTrees(slice, slices, limit int) (grown int) {
	for lz := slice; lz < worldD; lz += slices {
		for lx := 0; lx < worldW; lx++ {
			if grown >= limit {
				return grown // a few leaves per tick: each one costs a relight
			}
			top := w.Height[lz*worldW+lx]
			for y := 2; y < top; y++ {
				b := w.getLocal(lx, y, lz)
				switch {
				case isLog(b):
					// A trunk cell with sky above and air beside it near the top regrows a canopy.
					if !isLog(w.getLocal(lx, y+1, lz)) || hash2(lx, y*31+lz, 4242) < 0.3 {
						for _, d := range [][3]int{{1, 0, 0}, {-1, 0, 0}, {0, 0, 1}, {0, 0, -1}, {0, 1, 0}} {
							nx, ny, nz := lx+d[0], y+d[1], lz+d[2]
							if w.getLocal(nx, ny, nz) == Air && w.sunLocal(nx, ny, nz) >= 10 && hash2(nx, ny*17+nz, int(w.Version)) < 0.25 {
								w.Set(nx+originX, ny, nz+originZ, w.leafFor(lx, y, lz, b))
								grown++
							}
						}
					}
				case b == Leaves:
					// Leaves next to a trunk ripen into fruit occasionally; leaves also spread one step out.
					nearLog := false
					for _, d := range [][3]int{{1, 0, 0}, {-1, 0, 0}, {0, 0, 1}, {0, 0, -1}, {0, -1, 0}, {0, 1, 0}} {
						if isLog(w.getLocal(lx+d[0], y+d[1], lz+d[2])) {
							nearLog = true
						}
					}
					if nearLog {
						if hash2(lx, y*13+lz, int(w.Version)+1) < 0.04 {
							w.Set(lx+originX, y, lz+originZ, FruitLeaves)
							grown++
						}
						for _, d := range [][3]int{{1, 0, 0}, {-1, 0, 0}, {0, 0, 1}, {0, 0, -1}} {
							nx, nz := lx+d[0], lz+d[1]
							if w.getLocal(nx, y, nz) == Air && w.sunLocal(nx, y, nz) >= 10 && hash2(nx, y*19+nz, int(w.Version)+2) < 0.06 {
								w.Set(nx+originX, y, nz+originZ, Leaves)
								grown++
							}
						}
					}
				}
			}
		}
	}
	return grown
}

// GrowTree turns a sapling at world (x, y, z) into a tree if there is room.
func (w *World) GrowTree(x, y, z int) bool {
	lx, lz := x-originX, z-originZ
	sapling := w.getLocal(lx, y, lz)
	for yy := y; yy < y+6; yy++ {
		if b := w.getLocal(lx, yy, lz); b != Air && b != sapling && !b.IsLeaves() && b != TallGrass {
			return false
		}
	}
	set := func(x, y, z int, b Block) { w.Set(x+originX, y, z+originZ, b) }
	switch sapling {
	case AcaciaSapling:
		w.placeAcacia(lx, y, lz, set)
	case PalmSapling:
		w.placePalm(lx, y, lz, set)
	case CherrySapling:
		w.placeCherry(lx, y, lz, set)
	default:
		w.placeTree(lx, y, lz, set)
	}
	return true
}

func mathCos(a float64) float64 { return math.Cos(a) }
func mathSin(a float64) float64 { return math.Sin(a) }

func abs(a int) int {
	if a < 0 {
		return -a
	}
	return a
}

// ---------- block access ----------

// The world wraps: x and z are taken modulo the world size, so there are no
// edges. Only y is bounded.
func inLocal(x, y, z int) bool { return y >= 0 && y < worldH }

func wrapX(x int) int { return ((x % worldW) + worldW) % worldW }
func wrapZ(z int) int { return ((z % worldD) + worldD) % worldD }

// WrapPos moves a world position into the canonical range.
func WrapPos(p rl.Vector3) rl.Vector3 {
	for p.X < float32(originX) {
		p.X += float32(worldW)
	}
	for p.X >= float32(originX)+float32(worldW) {
		p.X -= float32(worldW)
	}
	for p.Z < float32(originZ) {
		p.Z += float32(worldD)
	}
	for p.Z >= float32(originZ)+float32(worldD) {
		p.Z -= float32(worldD)
	}
	return p
}

// WrapDelta returns a - b along the shortest path across the wrapped world.
func WrapDelta(a, b rl.Vector3) rl.Vector3 {
	d := rl.Vector3Subtract(a, b)
	for d.X > float32(worldW)/2 {
		d.X -= float32(worldW)
	}
	for d.X < -float32(worldW)/2 {
		d.X += float32(worldW)
	}
	for d.Z > float32(worldD)/2 {
		d.Z -= float32(worldD)
	}
	for d.Z < -float32(worldD)/2 {
		d.Z += float32(worldD)
	}
	return d
}

// WrapDist is the shortest distance between two positions across the wrap.
func WrapDist(a, b rl.Vector3) float32 { return rl.Vector3Length(WrapDelta(a, b)) }

// Near returns pos shifted by whole worlds so it is closest to ref (for drawing).
func Near(pos, ref rl.Vector3) rl.Vector3 {
	d := WrapDelta(pos, ref)
	return rl.NewVector3(ref.X+d.X, pos.Y, ref.Z+d.Z)
}

func (w *World) getLocal(x, y, z int) Block {
	if !inLocal(x, y, z) {
		return Air
	}
	return w.Blocks[(y*worldD+wrapZ(z))*worldW+wrapX(x)]
}

func (w *World) setLocal(x, y, z int, b Block) {
	if inLocal(x, y, z) {
		w.Blocks[(y*worldD+wrapZ(z))*worldW+wrapX(x)] = b
	}
}

func (w *World) recomputeHeight(x, z int) {
	x, z = wrapX(x), wrapZ(z)
	top, ground := 0, 0
	for y := worldH - 1; y >= 0; y-- {
		b := w.getLocal(x, y, z)
		if b == Air {
			continue
		}
		if top == 0 {
			top = y + 1
		}
		if blocks[b].Solid {
			ground = y + 1
			break
		}
	}
	w.Height[z*worldW+x] = top
	w.Ground[z*worldW+x] = ground
}

// Get returns the block at world coordinates (Air outside the volume).
func (w *World) Get(x, y, z int) Block { return w.getLocal(x-originX, y, z-originZ) }

// InBounds reports whether world block coordinates are inside the volume.
func (w *World) InBounds(x, y, z int) bool { return inLocal(x-originX, y, z-originZ) }

// Solid is the collision query: everything below y=0 is a wall; x and z wrap.
func (w *World) Solid(x, y, z int) bool {
	if y < 0 {
		return true
	}
	if y >= worldH {
		return false
	}
	i := (y*worldD+wrapZ(z-originZ))*worldW + wrapX(x-originX)
	if blocks[w.Blocks[i]].Solid {
		return true
	}
	// A fence is a block and a half tall: nothing walks or jumps through the cell above it.
	return y > 0 && w.Blocks[i-worldD*worldW] == Fence
}

// IsWater reports whether a world cell holds water.
func (w *World) IsWater(x, y, z int) bool { return w.Get(x, y, z) == Water }

// WaterAt reports whether a world position is inside water.
func (w *World) WaterAt(p rl.Vector3) bool { return w.IsWater(floorI(p.X), floorI(p.Y), floorI(p.Z)) }

// LavaAt reports whether a world position is inside lava.
func (w *World) LavaAt(p rl.Vector3) bool {
	return w.Get(floorI(p.X), floorI(p.Y), floorI(p.Z)) == Lava
}

// LiquidAt reports whether a world position is inside water or lava.
func (w *World) LiquidAt(p rl.Vector3) bool {
	return w.Get(floorI(p.X), floorI(p.Y), floorI(p.Z)).Liquid()
}

// TouchesBlock reports whether a box (feet at pos) overlaps or presses against a block type.
func (w *World) TouchesBlock(pos rl.Vector3, hw, h float32, b Block) bool {
	const m = 0.02
	for y := floorI(pos.Y); y <= floorI(pos.Y+h); y++ {
		for z := floorI(pos.Z - hw - m); z <= floorI(pos.Z+hw+m); z++ {
			for x := floorI(pos.X - hw - m); x <= floorI(pos.X+hw+m); x++ {
				if w.Get(x, y, z) == b {
					return true
				}
			}
		}
	}
	return false
}

// BlockAt returns the block containing a world position.
func (w *World) BlockAt(p rl.Vector3) Block { return w.Get(floorI(p.X), floorI(p.Y), floorI(p.Z)) }

// Set changes a block, updates the column heights and marks chunk meshes dirty.
func (w *World) Set(x, y, z int, b Block) {
	lx, lz := wrapX(x-originX), wrapZ(z-originZ)
	if !inLocal(lx, y, lz) {
		return
	}
	old := w.Blocks[(y*worldD+lz)*worldW+lx]
	w.Blocks[(y*worldD+lz)*worldW+lx] = b
	w.recomputeHeight(lx, lz)
	w.Version++
	if len(w.navDirty) < 4096 {
		w.navDirty = append(w.navDirty, int32(lz*worldW+lx))
	} else {
		w.navAll = true
	}
	cx, cz := lx/chunkSize, lz/chunkSize
	if !lightAlike(old, b) {
		// Only changes that can alter light queue a relight; grass turning to
		// dirt or wheat ripening just re-mesh.
		eb := w.relight[cz*w.ncx+cx]
		if eb == nil {
			eb = &editBox{lx, y, lz, lx + 1, y + 1, lz + 1}
			w.relight[cz*w.ncx+cx] = eb
		} else {
			eb.x0, eb.y0, eb.z0 = min(eb.x0, lx), min(eb.y0, y), min(eb.z0, lz)
			eb.x1, eb.y1, eb.z1 = max(eb.x1, lx+1), max(eb.y1, y+1), max(eb.z1, lz+1)
		}
	}
	if w.OnSet != nil {
		w.OnSet(lx+originX, y, lz+originZ, b)
	}
	mark := func(i, j int) {
		i, j = wrapI(i, w.ncx), wrapI(j, w.ncz)
		w.chunks[j*w.ncx+i].dirty = true
	}
	mark(cx, cz)
	// Ambient occlusion reaches one block into neighbouring chunks.
	if lx%chunkSize <= 1 {
		mark(cx-1, cz)
	}
	if lx%chunkSize >= chunkSize-2 {
		mark(cx+1, cz)
	}
	if lz%chunkSize <= 1 {
		mark(cx, cz-1)
	}
	if lz%chunkSize >= chunkSize-2 {
		mark(cx, cz+1)
	}
}

// SurfaceY returns the feet level of the highest solid block in a world column.
func (w *World) SurfaceY(x, z int) int {
	return w.Ground[wrapZ(z-originZ)*worldW+wrapX(x-originX)]
}

// SkyExposed reports whether nothing opaque sits above the given world position.
func (w *World) SkyExposed(p rl.Vector3) bool {
	x, z := floorI(p.X), floorI(p.Z)
	for y := floorI(p.Y) + 1; y < worldH; y++ {
		if w.Get(x, y, z).Opaque() {
			return false
		}
	}
	return true
}

func floorI(v float32) int { return int(math.Floor(float64(v))) }

// SpawnPoint is the player start, on the surface at the world centre.
func (w *World) SpawnPoint() rl.Vector3 {
	return rl.NewVector3(0.5, float32(w.SurfaceY(0, 0)), 0.5)
}

// ---------- rendering ----------

type faceDef struct {
	n, u, v [3]int // outward normal and the two tangent axes (u x v = n)
	shade   float32
}

var faces = [6]faceDef{
	{[3]int{0, 1, 0}, [3]int{1, 0, 0}, [3]int{0, 0, -1}, 1.0},
	{[3]int{0, -1, 0}, [3]int{1, 0, 0}, [3]int{0, 0, 1}, 0.5},
	{[3]int{1, 0, 0}, [3]int{0, 0, -1}, [3]int{0, 1, 0}, 0.8},
	{[3]int{-1, 0, 0}, [3]int{0, 0, 1}, [3]int{0, 1, 0}, 0.8},
	{[3]int{0, 0, 1}, [3]int{1, 0, 0}, [3]int{0, 1, 0}, 0.66},
	{[3]int{0, 0, -1}, [3]int{-1, 0, 0}, [3]int{0, 1, 0}, 0.66},
}

var faceCorners = [4][2]int{{-1, -1}, {1, -1}, {1, 1}, {-1, 1}}

// cornerLight is the smoothed light at a face corner: sunlight and block light in 0..1.
type cornerLight struct{ sun, blk float32 }

var fullLight = [4]cornerLight{{1, 1}, {1, 1}, {1, 1}, {1, 1}}
var unitBox = [2][3]float32{{0, 0, 0}, {1, 1, 1}}
var waterBox = [2][3]float32{{0, 0, 0}, {1, 0.875, 1}}

// emitFace appends one textured block face to a mesh buffer. The vertex colour
// encodes lighting for the terrain shader: R = sunlight, G = block light,
// B = directional shade x ambient occlusion. box gives the face extents within the cell.
func (m *meshBuf) emitFace(f *faceDef, b Block, x, y, z float32, box [2][3]float32, tint float32, ao [4]int, light [4]cornerLight) {
	face := 1
	if f.n[1] > 0 {
		face = 0
	} else if f.n[1] < 0 {
		face = 2
	}
	u0, v0, u1, v1 := tileUV(b, face)
	m.emitFaceUV(f, u0, v0, u1, v1, x, y, z, box, tint, ao, light)
}

// emitFaceUV is emitFace with an explicit texture rectangle (character skins use it too).
func (m *meshBuf) emitFaceUV(f *faceDef, u0, v0, u1, v1, x, y, z float32, box [2][3]float32, tint float32, ao [4]int, light [4]cornerLight) {
	var pos [4][3]float32
	var uv [4][2]float32
	var shade [4]uint8
	for k, c := range faceCorners {
		su, sv := float32(c[0]), float32(c[1])
		for a := 0; a < 3; a++ {
			t := 0.5 + float32(f.n[a])*0.5 + su*float32(f.u[a])*0.5 + sv*float32(f.v[a])*0.5
			pos[k][a] = lerp(box[0][a], box[1][a], t)
		}
		pos[k][0] += x
		pos[k][1] += y
		pos[k][2] += z
		if m.wobble && styleWobble() {
			wobbleCorner(&pos[k])
		}
		uv[k] = [2]float32{lerp(u0, u1, (su+1)/2), lerp(v0, v1, (1-sv)/2)}
		bright := 0.55 + 0.45*float32(ao[k])/3
		shade[k] = uint8(clamp(f.shade*tint*bright*255, 0, 255))
	}
	// Four corners per face; the shared index pattern (0,1,2, 0,2,3) makes the
	// two triangles. Starting from corner 1 instead of 0 flips the diagonal,
	// which keeps ambient occlusion from smearing across the wrong pair.
	order := [4]int{0, 1, 2, 3}
	if ao[0]+ao[2] < ao[1]+ao[3] {
		order = [4]int{1, 2, 3, 0}
	}
	for _, i := range order {
		m.verts = append(m.verts, pos[i][0], pos[i][1], pos[i][2])
		m.norms = append(m.norms, float32(f.n[0]), float32(f.n[1]), float32(f.n[2]))
		m.uvs = append(m.uvs, uv[i][0], uv[i][1])
		m.cols = append(m.cols, uint8(light[i].sun*255), uint8(light[i].blk*255), shade[i], 255)
	}
}

// wobbleAmp is how far (in blocks) a block corner may be nudged off its grid
// position for the hand-drawn look.
const wobbleAmp = 0.075

// wobbleCorner nudges a whole-block corner by an offset hashed from its world
// position, so every face that meets there moves together and the mesh stays
// sealed across chunk and wrap seams. Corners inside a cell (torches, beds,
// ladders) are left alone.
func wobbleCorner(p *[3]float32) {
	ix, iy, iz := floorI(p[0]), floorI(p[1]), floorI(p[2])
	if float32(ix) != p[0] || float32(iy) != p[1] || float32(iz) != p[2] {
		return
	}
	wx, wz := wrapX(ix-originX), wrapZ(iz-originZ)
	p[0] += (hash2(wx, iy*977+wz, 601) - 0.5) * 2 * wobbleAmp
	p[1] += (hash2(wx, iy*977+wz, 602) - 0.5) * 2 * wobbleAmp
	p[2] += (hash2(wx, iy*977+wz, 603) - 0.5) * 2 * wobbleAmp
}

// biomeTintCode returns the vertex-alpha code the shader turns into a biome
// colour for foliage faces, or 255 for faces that keep their texture colour.
func biomeTintCode(b Block, f *faceDef, biome Biome) uint8 {
	foliage := b == Leaves || b == SpruceLeaves || b == EucLeaves || b == FruitLeaves || b == AcaciaLeaves || b == TallGrass || b == Sapling || (b == Grass && f.n[1] > 0)
	if !foliage {
		return 255
	}
	return 250 - uint8(biome)
}

// emitCross draws a plant as two diagonal quads of the given height, both
// sides, textured with the block's side tile and lit by its own cell.
func (m *meshBuf) emitCross(b Block, x, y, z, h float32, l cornerLight, tintCode uint8) {
	u0, v0, u1, v1 := tileUV(b, 1)
	shade := uint8(230)
	type quad struct{ ax, az, bx, bz float32 }
	quads := []quad{{0.15, 0.15, 0.85, 0.85}, {0.15, 0.85, 0.85, 0.15}}
	for _, q := range quads {
		for _, flip := range []bool{false, true} {
			ax, az, bx, bz := q.ax, q.az, q.bx, q.bz
			if flip {
				ax, az, bx, bz = bx, bz, ax, az
			}
			// Corners: bottom-a, bottom-b, top-b, top-a.
			pts := [4][3]float32{{x + ax, y, z + az}, {x + bx, y, z + bz}, {x + bx, y + h, z + bz}, {x + ax, y + h, z + az}}
			uvs := [4][2]float32{{u0, v1}, {u1, v1}, {u1, v0}, {u0, v0}}
			for i := range pts {
				m.verts = append(m.verts, pts[i][0], pts[i][1], pts[i][2])
				m.norms = append(m.norms, 0, 1, 0)
				m.uvs = append(m.uvs, uvs[i][0], uvs[i][1])
				m.cols = append(m.cols, uint8(l.sun*255), uint8(l.blk*255), shade, tintCode)
			}
		}
	}
}

// setLastFaceAlpha rewrites the alpha of the four vertices just emitted.
func (m *meshBuf) setLastFaceAlpha(a uint8) {
	for k := 1; k <= 4; k++ {
		m.cols[len(m.cols)-4*k+3] = a
	}
}

// Every mesh is a list of quads, four vertices each, drawn through one shared
// index pattern. Indexing cuts the vertices the GPU must process by a third
// compared with six per face. 16-bit indices reach 65536 vertices; the rare
// mesh that is larger is expanded to plain triangles instead.
const maxQuadVerts = 65532

var quadIndex = func() []uint16 {
	idx := make([]uint16, maxQuadVerts/4*6)
	for q := 0; q < maxQuadVerts/4; q++ {
		v := uint16(q * 4)
		copy(idx[q*6:], []uint16{v, v + 1, v + 2, v, v + 2, v + 3})
	}
	return idx
}()

// expand rewrites the quads as two triangles each, for a mesh too big to index.
func (m *meshBuf) expand() {
	n := len(m.verts) / 12
	verts := make([]float32, 0, n*18)
	norms := make([]float32, 0, n*18)
	uvs := make([]float32, 0, n*12)
	cols := make([]uint8, 0, n*24)
	for q := 0; q < n; q++ {
		for _, k := range [6]int{0, 1, 2, 0, 2, 3} {
			i := q*4 + k
			verts = append(verts, m.verts[i*3:i*3+3]...)
			norms = append(norms, m.norms[i*3:i*3+3]...)
			uvs = append(uvs, m.uvs[i*2:i*2+2]...)
			cols = append(cols, m.cols[i*4:i*4+4]...)
		}
	}
	m.verts, m.norms, m.uvs, m.cols = verts, norms, uvs, cols
}

// vertexBufs is one set of CPU vertex arrays. Chunk builds borrow a set from
// bufPool, fill it, upload it and hand it back, so a rebuild appends into
// memory that is already the right size instead of growing four slices from
// nothing (that growth was a sixth of all CPU time and most of the GC churn).
type vertexBufs struct {
	verts, norms, uvs []float32
	cols              []uint8
}

var bufPool []vertexBufs

// meshWorkers is how many chunks are meshed at once: the spare cores, up to eight.
var meshWorkers = max(1, min(runtime.NumCPU()-1, 8))

// reset readies the buffer for a build, borrowing arrays from the pool.
func (m *meshBuf) reset() {
	if m.verts == nil && len(bufPool) > 0 {
		b := bufPool[len(bufPool)-1]
		bufPool = bufPool[:len(bufPool)-1]
		m.verts, m.norms, m.uvs, m.cols = b.verts, b.norms, b.uvs, b.cols
	}
	m.verts, m.norms, m.uvs, m.cols = m.verts[:0], m.norms[:0], m.uvs[:0], m.cols[:0]
}

// release returns the build arrays to the pool once the GPU has its copy.
func (m *meshBuf) release() {
	if m.verts != nil {
		bufPool = append(bufPool, vertexBufs{m.verts[:0], m.norms[:0], m.uvs[:0], m.cols[:0]})
	}
	m.verts, m.norms, m.uvs, m.cols = nil, nil, nil, nil
}

func (m *meshBuf) free() {
	if m.loaded {
		m.mesh.Indices = nil // the shared index array is ours, not raylib's to free
		rl.UnloadMesh(&m.mesh)
		m.loaded = false
	}
	m.mesh = rl.Mesh{}
}

func (m *meshBuf) upload() {
	m.free()
	if len(m.verts) == 0 {
		m.release()
		return
	}
	if len(m.verts)/3 > maxQuadVerts {
		m.expand()
		m.mesh.VertexCount = int32(len(m.verts) / 3)
		m.mesh.TriangleCount = m.mesh.VertexCount / 3
	} else {
		m.mesh.VertexCount = int32(len(m.verts) / 3)
		m.mesh.TriangleCount = m.mesh.VertexCount / 2
		m.mesh.Indices = &quadIndex[0] // stays set: raylib draws indexed when it is non-nil
	}
	m.mesh.Vertices = &m.verts[0]
	m.mesh.Normals = &m.norms[0]
	m.mesh.Texcoords = &m.uvs[0]
	m.mesh.Colors = &m.cols[0]
	rl.UploadMesh(&m.mesh, false)
	m.loaded = true
	// The GPU has its copy. Drawing goes through the VAO and unloading only
	// needs the buffer ids, so the CPU arrays are not referenced again: clear
	// the pointers and give the arrays back for the next build.
	m.mesh.Vertices, m.mesh.Normals, m.mesh.Texcoords, m.mesh.Colors = nil, nil, nil, nil
	m.release()
}

// buildChunk rebuilds one chunk's meshes from start to finish.
func (w *World) buildChunk(ci, cj int, c *chunk) {
	w.prepChunk(c)
	w.meshChunk(ci, cj, c)
	w.uploadChunk(c)
}

// A build has three steps so that several chunks can be meshed at once:
// prepChunk borrows vertex arrays (main thread, the pool is not locked),
// meshChunk fills them (any goroutine: it only reads the world and writes
// its own chunk), and uploadChunk hands them to the GPU (main thread).
func (w *World) prepChunk(c *chunk) {
	for k := 0; k < chunkSections; k++ {
		c.opaque[k].reset()
		c.trans[k].reset()
	}
}

func (w *World) uploadChunk(c *chunk) {
	for k := 0; k < chunkSections; k++ {
		c.opaque[k].upload()
		c.trans[k].upload()
	}
}

// buildChunks meshes a batch in parallel and uploads the results in order.
func (w *World) buildChunks(batch []cand) {
	for _, k := range batch {
		w.prepChunk(k.c)
	}
	if len(batch) == 1 {
		w.meshChunk(batch[0].ci, batch[0].cj, batch[0].c)
	} else {
		var wg sync.WaitGroup
		for _, k := range batch {
			wg.Add(1)
			go func(k cand) {
				defer wg.Done()
				w.meshChunk(k.ci, k.cj, k.c)
			}(k)
		}
		wg.Wait()
	}
	for _, k := range batch {
		w.uploadChunk(k.c)
		k.c.dirty = false
	}
}

func (w *World) meshChunk(ci, cj int, c *chunk) {
	occ := func(x, y, z int) int {
		if w.getLocal(x, y, z).Opaque() {
			return 1
		}
		return 0
	}
	for lz := cj * chunkSize; lz < (cj+1)*chunkSize; lz++ {
		for lx := ci * chunkSize; lx < (ci+1)*chunkSize; lx++ {
			top := w.Height[lz*worldW+lx]
			for y := 0; y < top; y++ {
				b := w.getLocal(lx, y, lz)
				if b == Air {
					continue
				}
				info := &blocks[b]
				trans := info.Trans
				wx, wz := float32(lx+originX), float32(lz+originZ)
				if info.Tiny {
					// Small box lit by its own cell, no culling or occlusion.
					l := cornerLight{float32(w.sunLocal(lx, y, lz)) / 15, float32(w.blockLocal(lx, y, lz)) / 15}
					box := b.TinyBox()
					switch {
					case b == Ladder:
						box = w.ladderBox(lx, y, lz)
					case isDoor(b):
						box = w.doorBox(lx, y, lz, b)
					case b == TrapdoorOpen:
						box = w.openTrapdoorBox(lx, y, lz)
					}
					if info.Cross {
						c.opaque[y/sectionH].emitCross(b, wx, float32(y), wz, box[1][1], l, biomeTintCode(b, &faces[0], w.Biome[lz*worldW+lx]))
						continue
					}
					boxes := [][2][3]float32{box}
					if b == Fence {
						boxes = w.fenceBoxes(lx, y, lz)
					}
					for _, box := range boxes {
						for fi := range faces {
							c.opaque[y/sectionH].emitFace(&faces[fi], b, wx, float32(y), wz, box, 1, [4]int{3, 3, 3, 3}, [4]cornerLight{l, l, l, l})
							if code := biomeTintCode(b, &faces[fi], w.Biome[lz*worldW+lx]); code != 255 {
								c.opaque[y/sectionH].setLastFaceAlpha(code)
							}
						}
					}
					continue
				}
				// Subtle per-block tint variation gives a weathered look.
				tint := 0.93 + hash2(lx, y*131+lz, 7)*0.1
				box := unitBox
				if b == Water && w.getLocal(lx, y+1, lz) != Water {
					box = waterBox // the surface sits a little below the block top
				}
				for fi := range faces {
					f := &faces[fi]
					nx, ny, nz := lx+f.n[0], y+f.n[1], lz+f.n[2]
					nb := w.getLocal(nx, ny, nz)
					leafy := b.IsLeaves()
					if (nb.Opaque() && !(leafy && nb == b)) || (trans && nb == b) {
						continue
					}
					if leafy && nb == b && f.n[0]+f.n[1]+f.n[2] < 0 {
						// Two leaf blocks share this plane and both would draw it,
						// back to back. One is enough to give the canopy its depth.
						continue
					}
					ao := [4]int{3, 3, 3, 3}
					var light [4]cornerLight
					for k, cn := range faceCorners {
						cells := [4][3]int{
							{nx, ny, nz},
							{nx + cn[0]*f.u[0], ny + cn[0]*f.u[1], nz + cn[0]*f.u[2]},
							{nx + cn[1]*f.v[0], ny + cn[1]*f.v[1], nz + cn[1]*f.v[2]},
							{nx + cn[0]*f.u[0] + cn[1]*f.v[0], ny + cn[0]*f.u[1] + cn[1]*f.v[1], nz + cn[0]*f.u[2] + cn[1]*f.v[2]},
						}
						s1, s2, cr := occ(cells[1][0], cells[1][1], cells[1][2]), occ(cells[2][0], cells[2][1], cells[2][2]), occ(cells[3][0], cells[3][1], cells[3][2])
						if !trans {
							if s1 == 1 && s2 == 1 {
								ao[k] = 0
							} else {
								ao[k] = 3 - s1 - s2 - cr
							}
						}
						// Smooth lighting: average the open cells touching this corner.
						var sun, blk, n float32
						for _, cl := range cells {
							if w.getLocal(cl[0], cl[1], cl[2]).Opaque() {
								continue
							}
							sun += float32(w.sunLocal(cl[0], cl[1], cl[2]))
							blk += float32(w.blockLocal(cl[0], cl[1], cl[2]))
							n++
						}
						if n == 0 {
							n = 1
						}
						light[k] = cornerLight{sun / n / 15, blk / n / 15}
					}
					dst := c.opaque[y/sectionH]
					if trans {
						dst = c.trans[y/sectionH]
					}
					dst.emitFace(f, b, wx, float32(y), wz, box, tint, ao, light)
					if code := biomeTintCode(b, f, w.Biome[lz*worldW+lx]); code != 255 {
						dst.setLastFaceAlpha(code)
					}
				}
			}
		}
	}
}

// BlockMesh returns a cached unit cube mesh (centred on the origin) for a block type.
func (w *World) BlockMesh(b Block) *meshBuf {
	if m, ok := w.blockM[b]; ok {
		return m
	}
	m := &meshBuf{}
	box := unitBox
	if blocks[b].Tiny {
		box = b.TinyBox()
	}
	for fi := range faces {
		m.emitFace(&faces[fi], b, -0.5, -0.5, -0.5, box, 1, [4]int{3, 3, 3, 3}, fullLight)
	}
	m.upload()
	w.blockM[b] = m
	return m
}

const vertexShader = `#version 330
in vec3 vertexPosition;
in vec2 vertexTexCoord;
in vec3 vertexNormal;
in vec4 vertexColor;
uniform mat4 mvp;
uniform mat4 matModel;
out vec2 fragTexCoord;
out vec4 fragColor;
out vec3 fragPos;
out vec3 fragNormal;
void main() {
    fragTexCoord = vertexTexCoord;
    fragColor = vertexColor;
    fragPos = (matModel * vec4(vertexPosition, 1.0)).xyz;
    fragNormal = vertexNormal;
    gl_Position = mvp * vec4(vertexPosition, 1.0);
}`

// Terrain: vertex colour R = sunlight, G = block light, B = shade; colDiffuse scales the same channels.
const terrainFragment = `#version 330
in vec2 fragTexCoord;
in vec4 fragColor;
in vec3 fragPos;
in vec3 fragNormal;
uniform sampler2D texture0;
uniform vec4 colDiffuse;
uniform vec3 viewPos;
uniform vec3 fogColor;
uniform float fogStart;
uniform float fogEnd;
uniform float light;
uniform vec3 sunTint;
uniform vec3 sunDir;
uniform float flicker;
uniform vec4 tileInfo;    // atlas cell size (xy) and the padding offset to the tile inside it (zw), in uv units
uniform vec2 uvScroll;    // water animation, in tile units (0 for the opaque pass)
uniform float water;      // 1 in the translucent pass
uniform float cel;        // 0..1: how far lighting is quantised into bands
out vec4 finalColor;
// band snaps a lighting level to one of a few steps with a soft edge between them.
float band(float v) {
    float s = 4.0;
    float q = floor(v * s) / s;
    return mix(q, q + 1.0 / s, smoothstep(0.3, 0.7, fract(v * s)));
}
void main() {
    vec2 uv = fragTexCoord;
    if (uvScroll != vec2(0.0)) {
        vec2 inner = tileInfo.xy - 2.0 * tileInfo.zw;
        vec2 origin = floor(uv / tileInfo.xy) * tileInfo.xy + tileInfo.zw;
        uv = origin + fract((uv - origin) / inner + uvScroll) * inner;
    }
    vec4 t = texture(texture0, uv);
    // Vertex alpha below 254/255 is a biome tint code for foliage, not transparency.
    float code = fragColor.a * 255.0;
    float va = 1.0;
    vec3 biome = vec3(1.0);
    if (code < 253.5) {
        if (code > 249.5) biome = vec3(0.92, 1.0, 0.78);        // plains
        else if (code > 248.5) biome = vec3(0.72, 1.0, 0.66);   // forest
        else if (code > 247.5) biome = vec3(1.0, 0.92, 0.55);   // desert
        else if (code > 246.5) biome = vec3(0.66, 0.96, 0.88);  // taiga
        else if (code > 245.5) biome = vec3(0.95, 0.85, 0.55);  // outback
        else biome = vec3(0.6, 0.72, 0.5);                      // swamp
        t.rgb *= biome;
    } else {
        va = fragColor.a;
    }
    float a = t.a * va * colDiffuse.a;
    if (a < 0.02) discard;
    float sun = fragColor.r * colDiffuse.r * light;
    float blk = fragColor.g * colDiffuse.g * flicker;
    // Faces turned toward the sun catch more of it; the effect follows the sun through the day.
    float facing = 0.88 + 0.24 * max(dot(normalize(fragNormal), sunDir), 0.0);
    float bs = 0.03 + 0.97 * sun * sun * facing;
    float bb = 0.97 * pow(blk, 1.4); // torchlight falls off more gently than sunlight
    vec3 torchTint = vec3(1.15, 0.98, 0.72); // warm and a touch over-bright up close
    vec3 lit = max(sunTint * bs, torchTint * bb);
    if (cel > 0.0) {
        float lv = max(bs, bb);
        float bl = mix(lv, band(lv), cel);
        lit *= bl / max(lv, 0.001);
    }
    vec3 rgb = t.rgb * fragColor.b * colDiffuse.b * lit;
    if (water > 0.5) {
        // More reflective (opaque) at grazing angles, clearer looking straight down.
        vec3 v = normalize(viewPos - fragPos);
        float facing = abs(dot(normalize(fragNormal), v));
        a = clamp(a * (0.7 + 0.6 * (1.0 - facing)), 0.0, 1.0);
        rgb += vec3(0.06, 0.08, 0.1) * (1.0 - facing) * bs;
    }
    // Gentle grading: a touch more saturation and contrast.
    float lum = dot(rgb, vec3(0.299, 0.587, 0.114));
    rgb = mix(vec3(lum), rgb, 1.12);
    rgb = (rgb - 0.5) * 1.06 + 0.5;
    float f = clamp((distance(viewPos, fragPos) - fogStart) / (fogEnd - fogStart), 0.0, 1.0);
    finalColor = vec4(mix(rgb, fogColor, f), a);
}`

// Entities: plain textured colour with fog; brightness is applied by the caller.
const entityFragment = `#version 330
in vec2 fragTexCoord;
in vec4 fragColor;
in vec3 fragPos;
uniform sampler2D texture0;
uniform vec4 colDiffuse;
uniform vec3 viewPos;
uniform vec3 fogColor;
uniform float fogStart;
uniform float fogEnd;
out vec4 finalColor;
void main() {
    vec4 c = texture(texture0, fragTexCoord) * fragColor * colDiffuse;
    if (c.a < 0.02) discard;
    float f = clamp((distance(viewPos, fragPos) - fogStart) / (fogEnd - fogStart), 0.0, 1.0);
    finalColor = vec4(mix(c.rgb, fogColor, f), c.a);
}`

// loadAtlas builds the block atlas for the current art style and uploads it.
func (w *World) loadAtlas() {
	atlas := buildAtlas()
	w.tex = rl.LoadTextureFromImage(rl.NewImage(atlas.Pix, int32(atlas.Rect.Dx()), int32(atlas.Rect.Dy()), 1, rl.UncompressedR8g8b8a8))
	rl.GenTextureMipmaps(&w.tex)
	if settings.Style == StylePixel {
		// Crisp pixels up close, mipmapped in the distance so far-off blocks stop shimmering.
		rl.TextureParameters(w.tex.ID, rl.TextureMinFilter, rl.TextureFilterNearestMipLinear)
		rl.TextureParameters(w.tex.ID, rl.TextureMagFilter, 0x2600) // GL_NEAREST
	} else {
		// Painterly tiles are meant to blur softly: trilinear filtering.
		rl.TextureParameters(w.tex.ID, rl.TextureMinFilter, 0x2703) // GL_LINEAR_MIPMAP_LINEAR
		rl.TextureParameters(w.tex.ID, rl.TextureMagFilter, rl.TextureFilterLinear)
	}
	w.mat.GetMap(rl.MapDiffuse).Texture = w.tex
	if w.shaderOK {
		aw, ah := float32(atlasTiles*atlasCell), float32(atlasRows*atlasCell)
		rl.SetShaderValue(w.shader, w.locTile, []float32{float32(atlasCell) / aw, float32(atlasCell) / ah, atlasPad / aw, atlasPad / ah}, rl.ShaderUniformVec4)
	}
}

// Restyle switches the art style on a live world: a new atlas, fresh item
// cube meshes (their texture rectangles moved) and every chunk rebuilt, all
// in view at once so the world does not fill in piecemeal.
func (w *World) Restyle() {
	if w.gpu {
		rl.UnloadTexture(w.tex)
		w.loadAtlas()
		for _, m := range w.blockM {
			m.free()
		}
		w.blockM = map[Block]*meshBuf{}
	}
	for _, c := range w.chunks {
		c.dirty = true
	}
	w.warmed = false
}

func (w *World) initGPU() {
	w.gpu = true
	w.mat = rl.LoadMaterialDefault()
	w.loadAtlas()
	w.mat.GetMap(rl.MapDiffuse).Texture = w.tex
	w.shader = rl.LoadShaderFromMemory(vertexShader, terrainFragment)
	w.eshader = rl.LoadShaderFromMemory(vertexShader, entityFragment)
	if rl.IsShaderValid(w.shader) && rl.IsShaderValid(w.eshader) {
		w.shaderOK = true
		w.mat.Shader = w.shader
		w.locView = rl.GetShaderLocation(w.shader, "viewPos")
		w.locFog = rl.GetShaderLocation(w.shader, "fogColor")
		w.locFogS = rl.GetShaderLocation(w.shader, "fogStart")
		w.locFogE = rl.GetShaderLocation(w.shader, "fogEnd")
		w.locLight = rl.GetShaderLocation(w.shader, "light")
		w.locSun = rl.GetShaderLocation(w.shader, "sunTint")
		w.locSunD = rl.GetShaderLocation(w.shader, "sunDir")
		w.locFlick = rl.GetShaderLocation(w.shader, "flicker")
		w.locTile = rl.GetShaderLocation(w.shader, "tileInfo")
		w.locScrol = rl.GetShaderLocation(w.shader, "uvScroll")
		w.locWater = rl.GetShaderLocation(w.shader, "water")
		w.locCel = rl.GetShaderLocation(w.shader, "cel")
		aw, ah := float32(atlasTiles*atlasCell), float32(atlasRows*atlasCell)
		rl.SetShaderValue(w.shader, w.locTile, []float32{float32(atlasCell) / aw, float32(atlasCell) / ah, atlasPad / aw, atlasPad / ah}, rl.ShaderUniformVec4)
		w.elocView = rl.GetShaderLocation(w.eshader, "viewPos")
		w.elocFog = rl.GetShaderLocation(w.eshader, "fogColor")
		w.elocFogS = rl.GetShaderLocation(w.eshader, "fogStart")
		w.elocFogE = rl.GetShaderLocation(w.eshader, "fogEnd")
	}
}

// Unload frees the GPU resources of a world that is being replaced.
func (w *World) Unload() {
	for _, c := range w.chunks {
		for k := 0; k < chunkSections; k++ {
			c.opaque[k].free()
			c.trans[k].free()
		}
	}
	for _, m := range w.blockM {
		m.free()
	}
	skins.Unload()
	if w.gpu {
		rl.UnloadTexture(w.tex)
		if w.shaderOK {
			rl.UnloadShader(w.shader)
			rl.UnloadShader(w.eshader)
		}
		w.gpu = false
	}
}

// Atlas returns the block texture atlas (for HUD icons).
func (w *World) Atlas() rl.Texture2D {
	if !w.gpu {
		w.initGPU()
	}
	return w.tex
}

// SetEnv pushes camera, fog and daylight to the shader. Call once per frame before drawing.
func (w *World) SetEnv(cam rl.Camera3D, env Env) {
	if !w.gpu {
		w.initGPU()
	}
	if !w.shaderOK {
		return
	}
	rl.SetShaderValue(w.shader, w.locView, []float32{cam.Position.X, cam.Position.Y, cam.Position.Z}, rl.ShaderUniformVec3)
	rl.SetShaderValue(w.shader, w.locFog, []float32{float32(env.Fog.R) / 255, float32(env.Fog.G) / 255, float32(env.Fog.B) / 255}, rl.ShaderUniformVec3)
	rl.SetShaderValue(w.shader, w.locFogS, []float32{env.FogStart}, rl.ShaderUniformFloat)
	rl.SetShaderValue(w.shader, w.locFogE, []float32{env.FogEnd}, rl.ShaderUniformFloat)
	rl.SetShaderValue(w.shader, w.locLight, []float32{env.Light}, rl.ShaderUniformFloat)
	rl.SetShaderValue(w.shader, w.locSun, env.SunTint[:], rl.ShaderUniformVec3)
	rl.SetShaderValue(w.shader, w.locSunD, []float32{env.SunDir.X, env.SunDir.Y, env.SunDir.Z}, rl.ShaderUniformVec3)
	rl.SetShaderValue(w.shader, w.locFlick, []float32{env.Flicker}, rl.ShaderUniformFloat)
	rl.SetShaderValue(w.shader, w.locScrol, []float32{0, 0}, rl.ShaderUniformVec2)
	rl.SetShaderValue(w.shader, w.locWater, []float32{0}, rl.ShaderUniformFloat)
	cel := float32(0)
	if styleInk() {
		cel = 0.7
	}
	rl.SetShaderValue(w.shader, w.locCel, []float32{cel}, rl.ShaderUniformFloat)
	w.envTime = env.Time
	w.envFogEnd = env.FogEnd
	rl.SetShaderValue(w.eshader, w.elocView, []float32{cam.Position.X, cam.Position.Y, cam.Position.Z}, rl.ShaderUniformVec3)
	rl.SetShaderValue(w.eshader, w.elocFog, []float32{float32(env.Fog.R) / 255, float32(env.Fog.G) / 255, float32(env.Fog.B) / 255}, rl.ShaderUniformVec3)
	rl.SetShaderValue(w.eshader, w.elocFogS, []float32{env.FogStart}, rl.ShaderUniformFloat)
	rl.SetShaderValue(w.eshader, w.elocFogE, []float32{env.FogEnd}, rl.ShaderUniformFloat)
}

// BeginShader/EndShader route raylib's immediate-mode shapes through the lit, fogged world shader.
func (w *World) BeginShader() {
	if w.shaderOK {
		rl.BeginShaderMode(w.eshader)
	}
}

func (w *World) EndShader() {
	if w.shaderOK {
		rl.EndShaderMode()
	}
}

// visibleChunks walks the chunks, each shifted by whole worlds so it sits as
// close to the camera as possible: that is what makes the wrap seamless.
// Per-frame render statistics for the debug overlay.
var renderStats struct {
	Chunks, Verts, Pending int
}

// visibleChunks walks the chunks in view, building at most a few dirty ones
// per frame (nearest first) so a new area streams in without a stall.
// visibleChunks calls fn for every chunk worth drawing. The opaque pass
// (build=true) also flushes lighting and rebuilds dirty chunks within the
// frame's time budget; the translucent pass that follows reuses its list, so
// a backlog never costs two rounds of builds in one frame.
func (w *World) visibleChunks(cam rl.Camera3D, build bool, fn func(c *chunk, m rl.Matrix)) {
	if !build {
		for _, k := range w.lastReady {
			fn(k.c, rl.MatrixTranslate(k.ox, 0, k.oz))
		}
		return
	}
	w.flushLight()
	fwd := rl.Vector3Normalize(rl.Vector3Subtract(cam.Target, cam.Position))
	ready, dirty := w.lastReady[:0], w.scratchDirty[:0]
	for cj := 0; cj < w.ncz; cj++ {
		for ci := 0; ci < w.ncx; ci++ {
			c := w.chunks[cj*w.ncx+ci]
			centre := rl.NewVector3(float32(ci*chunkSize+chunkSize/2+originX), worldH/2, float32(cj*chunkSize+chunkSize/2+originZ))
			near := Near(centre, cam.Position)
			ox, oz := near.X-centre.X, near.Z-centre.Z
			if rl.Vector3DotProduct(rl.Vector3Subtract(near, cam.Position), fwd) < -28 {
				continue
			}
			dx, dz := near.X-cam.Position.X, near.Z-cam.Position.Z
			d2 := dx*dx + dz*dz
			// Nothing beyond the fog is visible: skip (and never even build) those chunks.
			if limit := w.envFogEnd + chunkSize; limit > 0 && d2 > limit*limit {
				continue
			}
			k := cand{c, ci, cj, ox, oz, d2}
			if c.dirty {
				dirty = append(dirty, k)
			} else {
				ready = append(ready, k)
			}
		}
	}
	// Build the nearest dirty chunks within this frame's budget. The first
	// frame of a world builds everything in view (the load pause hides it),
	// and a large backlog (after a teleport or respawn) clears in about a
	// second rather than trickling in.
	// The budget is time, not a count: builds vary from a flat beach section
	// to a forest full of leaves, and a count of four could take 20 ms. At
	// least one chunk is built per frame so a backlog always drains; a large
	// one (a teleport, a respawn) gets a bigger slice so it clears in about a
	// second rather than trickling in.
	// Chunks are meshed in parallel batches: the CPU work of a batch costs
	// about what one chunk used to, so a backlog drains several times faster
	// inside the same budget. Uploads stay on this thread, which is what
	// limits a batch in normal play; a backlog or the first frame uses every
	// spare core.
	budget := time.Duration(quality().BuildMS * float32(time.Millisecond))
	batchMax := 3
	if len(dirty) > 24 {
		budget *= 3
		batchMax = meshWorkers
	}
	if !w.warmed {
		budget = time.Hour // the load pause hides the first frame's builds
		batchMax = meshWorkers
		w.warmed = true
	}
	sort.Slice(dirty, func(i, j int) bool { return dirty[i].d2 < dirty[j].d2 })
	start := time.Now()
	built := 0
	if perfTrace && len(dirty) > 24 {
		defer func() { rl.TraceLog(rl.LogInfo, "LOAD built %d chunks in %.0fms", built, ms(time.Since(start))) }()
	}
	for built < len(dirty) && (built == 0 || time.Since(start) < budget) {
		n := min(batchMax, meshWorkers, len(dirty)-built)
		batch := dirty[built : built+n]
		t0 := time.Now()
		w.buildChunks(batch)
		if d := time.Since(t0); d > 6*time.Millisecond && perfTrace {
			rl.TraceLog(rl.LogInfo, "BUILD batch of %d took %.1fms", n, ms(d))
		}
		perfBuilds += n
		built += n
		ready = append(ready, batch...)
	}
	renderStats.Pending = max(0, len(dirty)-built)
	renderStats.Chunks = len(ready)
	w.lastReady, w.scratchDirty = ready, dirty
	for _, k := range ready {
		fn(k.c, rl.MatrixTranslate(k.ox, 0, k.oz))
	}
}

// cand is a chunk in view: its drawing offset and distance from the camera.
type cand struct {
	c      *chunk
	ci, cj int
	ox, oz float32
	d2     float32
}

// sectionVisible decides whether a vertical section of a chunk at the given
// offset is worth drawing. The deep layer (below the surface terrain) is only
// drawn near the camera, or when the camera is itself underground.
func sectionVisible(k int, m rl.Matrix, cam rl.Camera3D, ci, cj int) bool {
	top := float32((k + 1) * sectionH)
	if top > float32(groundBase)+4 {
		return true // holds surface terrain
	}
	if cam.Position.Y < float32(groundBase)+6 {
		return true
	}
	cx := float32(ci*chunkSize+chunkSize/2+originX) + m.M12
	cz := float32(cj*chunkSize+chunkSize/2+originZ) + m.M14
	dx, dz := cx-cam.Position.X, cz-cam.Position.Z
	return dx*dx+dz*dz < 56*56
}

// Draw renders the opaque geometry of all chunks not fully behind the camera.
func (w *World) Draw(cam rl.Camera3D) {
	if !w.gpu {
		t0 := time.Now()
		w.initGPU()
		if perfTrace {
			rl.TraceLog(rl.LogInfo, "LOAD initGPU %.0fms", ms(time.Since(t0)))
		}
	}
	w.visibleChunks(cam, true, func(c *chunk, m rl.Matrix) {
		for k := 0; k < chunkSections; k++ {
			if c.opaque[k].loaded && sectionVisible(k, m, cam, c.ci, c.cj) {
				renderStats.Verts += int(c.opaque[k].mesh.VertexCount)
				rl.DrawMesh(c.opaque[k].mesh, w.mat, m)
			}
		}
	})
}

// DrawTranslucent renders water and glass with animated, angle-dependent water.
func (w *World) DrawTranslucent(cam rl.Camera3D) {
	if w.shaderOK {
		t := w.envTime
		rl.SetShaderValue(w.shader, w.locScrol, []float32{float32(math.Sin(float64(t*0.7))) * 0.15, t * 0.08}, rl.ShaderUniformVec2)
		rl.SetShaderValue(w.shader, w.locWater, []float32{1}, rl.ShaderUniformFloat)
	}
	rl.DisableBackfaceCulling()
	w.visibleChunks(cam, false, func(c *chunk, m rl.Matrix) {
		for k := 0; k < chunkSections; k++ {
			if c.trans[k].loaded && sectionVisible(k, m, cam, c.ci, c.cj) {
				renderStats.Verts += int(c.trans[k].mesh.VertexCount)
				rl.DrawMesh(c.trans[k].mesh, w.mat, m)
			}
		}
	})
	rl.EnableBackfaceCulling()
	if w.shaderOK {
		rl.SetShaderValue(w.shader, w.locScrol, []float32{0, 0}, rl.ShaderUniformVec2)
		rl.SetShaderValue(w.shader, w.locWater, []float32{0}, rl.ShaderUniformFloat)
	}
}

// DrawBlockAt draws a block-textured cube with the given transform (item drops),
// lit by the cell it sits in.
func (w *World) DrawBlockAt(b Block, transform rl.Matrix) {
	lx, y, lz := floorI(transform.M12)-originX, floorI(transform.M13), floorI(transform.M14)-originZ
	m := w.mat.GetMap(rl.MapDiffuse)
	m.Color = rl.NewColor(uint8(w.sunLocal(lx, y, lz)*17), uint8(w.blockLocal(lx, y, lz)*17), 255, 255)
	rl.DrawMesh(w.BlockMesh(b).mesh, w.mat, transform)
	m.Color = rl.White
}

// ---------- collision ----------

type MoveResult struct {
	Ground, Ceiling, Wall bool
}

func (w *World) boxSolid(x0, y0, z0, x1, y1, z1 float32) bool {
	const eps = 1e-4
	for y := floorI(y0); y <= floorI(y1-eps); y++ {
		for z := floorI(z0); z <= floorI(z1-eps); z++ {
			for x := floorI(x0); x <= floorI(x1-eps); x++ {
				if top := w.cellTop(x, y, z); top > 0 && y0 < float32(y)+top {
					return true
				}
			}
		}
	}
	return false
}

// cellTop is how high the solid part of a cell reaches above its floor: 0 for
// an empty cell, 1 for a block, less for a closed trapdoor and for the half
// block of air above a fence post.
func (w *World) cellTop(x, y, z int) float32 {
	if y < 0 {
		return 1
	}
	if y >= worldH {
		return 0
	}
	switch b := w.Get(x, y, z); {
	case b == TrapdoorClosed:
		return trapdoorH
	case blocks[b].Solid:
		return 1
	case y > 0 && w.Get(x, y-1, z) == Fence:
		return 0.5
	}
	return 0
}

// rowTop is the highest cellTop among the cells a box footprint covers in row y.
func (w *World) rowTop(x0, z0, x1, z1 float32, y int) float32 {
	const eps = 1e-4
	var top float32
	for z := floorI(z0); z <= floorI(z1-eps); z++ {
		for x := floorI(x0); x <= floorI(x1-eps); x++ {
			top = max(top, w.cellTop(x, y, z))
		}
	}
	return top
}

// HasGround reports whether a box with feet at pos rests on something solid.
func (w *World) HasGround(pos rl.Vector3, hw float32) bool {
	return w.boxSolid(pos.X-hw, pos.Y-0.05, pos.Z-hw, pos.X+hw, pos.Y, pos.Z+hw)
}

// MoveBox moves an axis-aligned box (feet at pos, half width hw, height h) by
// delta, sliding along solid blocks. With stepUp, a blocked horizontal move
// may climb one block when standing on the ground.
func (w *World) MoveBox(pos rl.Vector3, hw, h float32, delta rl.Vector3, stepUp bool) (rl.Vector3, MoveResult) {
	const eps = 0.001
	var res MoveResult
	solid := func(p rl.Vector3) bool {
		return w.boxSolid(p.X-hw, p.Y, p.Z-hw, p.X+hw, p.Y+h, p.Z+hw)
	}

	// Vertical.
	oldY := pos.Y
	pos.Y += delta.Y
	if solid(pos) {
		if delta.Y < 0 {
			for y := floorI(oldY); y >= floorI(pos.Y); y-- {
				if w.boxSolid(pos.X-hw, float32(y), pos.Z-hw, pos.X+hw, float32(y+1), pos.Z+hw) {
					pos.Y = float32(y) + w.rowTop(pos.X-hw, pos.Z-hw, pos.X+hw, pos.Z+hw, y)
					break
				}
			}
			res.Ground = true
		} else {
			for y := floorI(oldY + h); y <= floorI(pos.Y+h); y++ {
				if w.boxSolid(pos.X-hw, float32(y), pos.Z-hw, pos.X+hw, float32(y+1), pos.Z+hw) {
					pos.Y = float32(y) - h - eps
					break
				}
			}
			res.Ceiling = true
		}
	} else if delta.Y <= 0 {
		// Standing exactly on a surface counts as grounded.
		probe := pos
		probe.Y -= 0.01
		res.Ground = solid(probe)
	}

	// Horizontal, one axis at a time.
	moveAxis := func(dx, dz float32) {
		if dx == 0 && dz == 0 {
			return
		}
		try := pos
		try.X += dx
		try.Z += dz
		if !solid(try) {
			pos = try
			return
		}
		if stepUp && res.Ground {
			// Climb onto whatever blocks the way: a full block, or just a trapdoor's slab.
			up := try
			up.Y = float32(floorI(try.Y)) + w.rowTop(try.X-hw, try.Z-hw, try.X+hw, try.Z+hw, floorI(try.Y))
			if up.Y <= try.Y {
				up.Y = try.Y + 1
			}
			if !solid(up) {
				pos = up
				return
			}
		}
		res.Wall = true
		if dx > 0 {
			pos.X = float32(floorI(try.X+hw)) - hw - eps
		} else if dx < 0 {
			pos.X = float32(floorI(try.X-hw)+1) + hw + eps
		}
		if dz > 0 {
			pos.Z = float32(floorI(try.Z+hw)) - hw - eps
		} else if dz < 0 {
			pos.Z = float32(floorI(try.Z-hw)+1) + hw + eps
		}
	}
	moveAxis(delta.X, 0)
	moveAxis(0, delta.Z)
	return pos, res
}

// PointFree reports whether a box of the given half width standing at pos is clear of blocks.
func (w *World) PointFree(pos rl.Vector3, radius float32) bool {
	return !w.boxSolid(pos.X-radius, pos.Y, pos.Z-radius, pos.X+radius, pos.Y+1.8, pos.Z+radius)
}

// ---------- ray queries ----------

// RayHit describes the first solid block along a ray and the face it entered through.
type RayHit struct {
	Hit        bool
	Dist       float32
	X, Y, Z    int // block
	NX, NY, NZ int // face normal (points to the adjacent air cell)
}

// RayCast walks the voxel grid (Amanatides & Woo) up to maxD units, stopping at solid blocks.
func (w *World) RayCast(o, d rl.Vector3, maxD float32) RayHit {
	return w.rayCast(o, d, maxD, w.solidBlock)
}

// solidBlock is Solid without the fence's extra half block, so rays aim at blocks that exist.
func (w *World) solidBlock(x, y, z int) bool {
	return y < 0 || blocks[w.Get(x, y, z)].Solid
}

// RayCastAny is RayCast for aiming: it also stops at non-solid blocks such as torches.
// RayCastLiquid also stops at water and lava, for buckets.
func (w *World) RayCastLiquid(o, d rl.Vector3, maxD float32) RayHit {
	return w.rayCast(o, d, maxD, func(x, y, z int) bool {
		return w.solidBlock(x, y, z) || w.Get(x, y, z) != Air
	})
}

func (w *World) RayCastAny(o, d rl.Vector3, maxD float32) RayHit {
	return w.rayCast(o, d, maxD, func(x, y, z int) bool {
		if w.solidBlock(x, y, z) {
			return true
		}
		b := w.Get(x, y, z)
		return b != Air && b != Water
	})
}

func (w *World) rayCast(o, d rl.Vector3, maxD float32, hit func(x, y, z int) bool) RayHit {
	d = rl.Vector3Normalize(d)
	x, y, z := floorI(o.X), floorI(o.Y), floorI(o.Z)
	step := [3]int{sign(d.X), sign(d.Y), sign(d.Z)}
	dir := [3]float32{d.X, d.Y, d.Z}
	orig := [3]float32{o.X, o.Y, o.Z}
	cell := [3]int{x, y, z}
	var tMax, tDelta [3]float32
	for i := 0; i < 3; i++ {
		if dir[i] == 0 {
			tMax[i] = float32(math.Inf(1))
			tDelta[i] = float32(math.Inf(1))
			continue
		}
		tDelta[i] = float32(math.Abs(float64(1 / dir[i])))
		var next float32
		if step[i] > 0 {
			next = float32(cell[i]+1) - orig[i]
		} else {
			next = orig[i] - float32(cell[i])
		}
		tMax[i] = next * tDelta[i]
	}
	var normal [3]int
	t := float32(0)
	for t <= maxD {
		if cell[1] < 0 || cell[1] >= worldH && step[1] >= 0 {
			break
		}
		if hit(cell[0], cell[1], cell[2]) {
			return RayHit{true, t, cell[0], cell[1], cell[2], normal[0], normal[1], normal[2]}
		}
		axis := 0
		if tMax[1] < tMax[axis] {
			axis = 1
		}
		if tMax[2] < tMax[axis] {
			axis = 2
		}
		t = tMax[axis]
		tMax[axis] += tDelta[axis]
		cell[axis] += step[axis]
		normal = [3]int{}
		normal[axis] = -step[axis]
	}
	return RayHit{}
}

func sign(v float32) int {
	if v > 0 {
		return 1
	}
	if v < 0 {
		return -1
	}
	return 0
}

// RayDistance returns the distance to the first block hit, or +Inf.
func (w *World) RayDistance(ray rl.Ray) float32 {
	if h := w.RayCast(ray.Position, ray.Direction, 200); h.Hit {
		return h.Dist
	}
	return float32(math.Inf(1))
}

// RandomFreePoint finds a dry surface spawn point at least minDist from `from`.
func (w *World) RandomFreePoint(from rl.Vector3, minDist float32) rl.Vector3 {
	for i := 0; i < 300; i++ {
		lx, lz := rand.Intn(worldW-4)+2, rand.Intn(worldD-4)+2
		h := w.Ground[lz*worldW+lx]
		if h != w.Height[lz*worldW+lx] || h <= seaLevel {
			continue // under water or under a canopy
		}
		if g := w.getLocal(lx, h-1, lz); g.IsLeaves() || isLog(g) {
			continue // on top of a tree
		}
		p := rl.NewVector3(float32(lx+originX)+0.5, float32(h), float32(lz+originZ)+0.5)
		if h+2 < worldH && w.getLocal(lx, h+1, lz) == Air && w.PointFree(p, 0.5) &&
			rl.Vector3Distance(rl.NewVector3(p.X, 0, p.Z), rl.NewVector3(from.X, 0, from.Z)) >= minDist {
			return p
		}
	}
	return rl.NewVector3(0.5, float32(w.SurfaceY(0, 0)), float32(originZ+3)+0.5)
}

// RandomDarkPoint is RandomFreePoint restricted to spots without torchlight,
// so a well-lit base keeps hostiles from rising inside it.
func (w *World) RandomDarkPoint(from rl.Vector3, minDist float32) (rl.Vector3, bool) {
	for i := 0; i < 40; i++ {
		p := w.RandomFreePoint(from, minDist)
		lx, lz := floorI(p.X)-originX, floorI(p.Z)-originZ
		if w.blockLocal(lx, floorI(p.Y), lz) < 8 {
			return p, true
		}
	}
	return rl.Vector3{}, false
}

func clamp(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
