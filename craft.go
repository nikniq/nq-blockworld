package main

import (
	"fmt"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Category groups recipes into the tabs of the crafting screen.
type Category uint8

const (
	CatBuilding Category = iota
	CatFurniture
	CatFood
	CatTools
	numCats
)

var catNames = [numCats]string{"Building", "Furniture", "Food", "Tools & Arms"}

type Ingredient struct {
	Block Block
	N     int
}

// Recipe turns inventory blocks into blocks, ammo or a better tool.
type Recipe struct {
	Name  string
	Out   Block
	Count int
	Ammo  int
	Sword int // tier granted
	Pick  int // tier granted
	Armor int // tier granted
	Cat   Category
	In    []Ingredient
}

var recipes = []Recipe{
	{Name: "Oak Planks x4", Out: Planks, Count: 4, In: []Ingredient{{Log, 1}}},
	{Name: "Planks x4 (birch)", Out: Planks, Count: 4, In: []Ingredient{{BirchLog, 1}}},
	{Name: "Planks x4 (acacia)", Out: Planks, Count: 4, In: []Ingredient{{AcaciaLog, 1}}},
	{Name: "Planks x4 (palm)", Out: Planks, Count: 4, In: []Ingredient{{PalmLog, 1}}},
	{Name: "Planks x4 (cherry)", Out: Planks, Count: 4, In: []Ingredient{{CherryLog, 1}}},
	{Name: "Planks x4 (eucalyptus)", Out: Planks, Count: 4, In: []Ingredient{{EucLog, 1}}},
	{Name: "Torch x4", Out: Torch, Count: 4, In: []Ingredient{{CoalOre, 1}, {Planks, 1}}},
	{Name: "Cooked Meat x2", Out: CookedMeat, Count: 2, Cat: CatFood, In: []Ingredient{{Meat, 2}, {CoalOre, 1}}},
	{Name: "Bread", Out: Bread, Count: 1, Cat: CatFood, In: []Ingredient{{WheatItem, 3}}},
	{Name: "Bow", Out: Bow, Count: 1, Cat: CatTools, In: []Ingredient{{Planks, 3}, {Wool, 2}}},
	{Name: "Door", Out: DoorClosed, Count: 1, In: []Ingredient{{Planks, 4}}},
	{Name: "Trapdoor x2", Out: TrapdoorClosed, Count: 2, In: []Ingredient{{Planks, 3}}},
	{Name: "Pumpkin Pie", Out: PumpkinPie, Count: 1, Cat: CatFood, In: []Ingredient{{Pumpkin, 1}, {WheatItem, 1}}},
	{Name: "Pumpkin Seeds x4", Out: PumpkinSeeds, Count: 4, Cat: CatFood, In: []Ingredient{{Pumpkin, 1}}},
	{Name: "Melon Seeds x2", Out: MelonSeeds, Count: 2, Cat: CatFood, In: []Ingredient{{MelonSlice, 1}}},
	{Name: "Chest", Out: ChestBlock, Count: 1, In: []Ingredient{{Planks, 8}}},
	{Name: "Fishing Rod", Out: Rod, Count: 1, Cat: CatTools, In: []Ingredient{{Planks, 3}, {Wool, 2}}},
	{Name: "Boat", Out: Boat, Count: 1, In: []Ingredient{{Planks, 5}}},
	{Name: "Village Flag", Out: FlagItem, Count: 1, In: []Ingredient{{Wool, 3}, {Planks, 2}, {GoldOre, 1}}},
	{Name: "Iron Ore x3 (smelt meteorite)", Out: IronOre, Count: 3, In: []Ingredient{{Meteorite, 1}, {CoalOre, 1}}},
	{Name: "Rifle ammo x30 (meteorite)", Ammo: 30, Cat: CatTools, In: []Ingredient{{Meteorite, 2}}},
	{Name: "Arrows x4", Out: ArrowItem, Count: 4, Cat: CatTools, In: []Ingredient{{Planks, 1}, {Gravel, 1}}},
	{Name: "TNT", Out: TNT, Count: 1, Cat: CatTools, In: []Ingredient{{Sand, 4}, {CoalOre, 4}}},
	{Name: "Ladder x4", Out: Ladder, Count: 4, In: []Ingredient{{Planks, 2}}},
	{Name: "Fence x4", Out: Fence, Count: 4, In: []Ingredient{{Planks, 3}}},
	{Name: "Bucket", Out: Bucket, Count: 1, Cat: CatTools, In: []Ingredient{{IronOre, 3}, {CoalOre, 1}}},
	{Name: "Bed", Out: Bed, Count: 1, Cat: CatFurniture, In: []Ingredient{{Planks, 3}, {Wool, 3}}},
	{Name: "Bunk Bed", Out: BunkBed, Count: 1, Cat: CatFurniture, In: []Ingredient{{Bed, 2}, {Planks, 2}}},
	{Name: "Table", Out: Table, Count: 1, Cat: CatFurniture, In: []Ingredient{{Planks, 3}}},
	{Name: "Chair x2", Out: Chair, Count: 2, Cat: CatFurniture, In: []Ingredient{{Planks, 3}}},
	{Name: "Flower Vase (poppy)", Out: Vase, Count: 1, Cat: CatFurniture, In: []Ingredient{{Flower, 2}, {RedSand, 1}}},
	{Name: "Flower Vase (dandelion)", Out: Vase, Count: 1, Cat: CatFurniture, In: []Ingredient{{FlowerYellow, 2}, {RedSand, 1}}},
	{Name: "Flower Vase (cornflower)", Out: Vase, Count: 1, Cat: CatFurniture, In: []Ingredient{{FlowerBlue, 2}, {RedSand, 1}}},
	{Name: "Flower Vase (daisy)", Out: Vase, Count: 1, Cat: CatFurniture, In: []Ingredient{{FlowerWhite, 2}, {RedSand, 1}}},
	{Name: "Flower Vase (tulip)", Out: Vase, Count: 1, Cat: CatFurniture, In: []Ingredient{{FlowerPink, 2}, {RedSand, 1}}},
	{Name: "Flower Vase (clay from sand)", Out: Vase, Count: 1, Cat: CatFurniture, In: []Ingredient{{Flower, 2}, {Sand, 2}}},
	{Name: "Bookshelf", Out: Bookshelf, Count: 1, Cat: CatFurniture, In: []Ingredient{{Planks, 6}}},
	{Name: "Lamp", Out: Lamp, Count: 1, Cat: CatFurniture, In: []Ingredient{{Torch, 1}, {Wool, 1}, {Planks, 1}}},
	{Name: "Rug x3", Out: Rug, Count: 3, Cat: CatFurniture, In: []Ingredient{{Wool, 2}}},
	{Name: "Red Rug x3", Out: RugRed, Count: 3, Cat: CatFurniture, In: []Ingredient{{Wool, 2}, {Flower, 1}}},
	{Name: "Blue Rug x3", Out: RugBlue, Count: 3, Cat: CatFurniture, In: []Ingredient{{Wool, 2}, {FlowerBlue, 1}}},
	{Name: "Sofa", Out: Sofa, Count: 1, Cat: CatFurniture, In: []Ingredient{{Planks, 2}, {Wool, 2}, {Flower, 1}}},
	{Name: "Fireplace", Out: Fireplace, Count: 1, Cat: CatFurniture, In: []Ingredient{{Cobble, 6}, {CoalOre, 1}}},
	{Name: "Painting", Out: Painting, Count: 1, Cat: CatFurniture, In: []Ingredient{{Planks, 2}, {Wool, 1}}},
	{Name: "Wall Clock", Out: Clock, Count: 1, Cat: CatFurniture, In: []Ingredient{{Planks, 2}, {GoldOre, 1}}},
	{Name: "Stone Bricks x4", Out: StoneBrick, Count: 4, In: []Ingredient{{Cobble, 4}}},
	{Name: "Glass x4", Out: Glass, Count: 4, In: []Ingredient{{Sand, 4}, {CoalOre, 1}}},
	{Name: "Gold Block", Out: GoldBlock, Count: 1, In: []Ingredient{{GoldOre, 2}, {CoalOre, 1}}},
	{Name: "Rifle ammo x16", Ammo: 16, Cat: CatTools, In: []Ingredient{{IronOre, 1}, {CoalOre, 1}}},
	{Name: "Rifle ammo x6", Ammo: 6, Cat: CatTools, In: []Ingredient{{Gravel, 3}, {CoalOre, 1}}},
	{Name: "Stone Sword", Sword: TierStone, Cat: CatTools, In: []Ingredient{{Cobble, 2}, {Planks, 1}}},
	{Name: "Iron Sword", Sword: TierIron, Cat: CatTools, In: []Ingredient{{IronOre, 2}, {CoalOre, 1}, {Planks, 1}}},
	{Name: "Diamond Sword", Sword: TierDiamond, Cat: CatTools, In: []Ingredient{{DiamondOre, 2}, {Planks, 1}}},
	{Name: "Stone Pickaxe", Pick: TierStone, Cat: CatTools, In: []Ingredient{{Cobble, 3}, {Planks, 2}}},
	{Name: "Iron Pickaxe", Pick: TierIron, Cat: CatTools, In: []Ingredient{{IronOre, 3}, {CoalOre, 1}, {Planks, 2}}},
	{Name: "Diamond Pickaxe", Pick: TierDiamond, Cat: CatTools, In: []Ingredient{{DiamondOre, 3}, {Planks, 2}}},
	{Name: "Leather Armour", Armor: 1, Cat: CatTools, In: []Ingredient{{Leather, 5}}},
	{Name: "Iron Armour", Armor: 2, Cat: CatTools, In: []Ingredient{{IronOre, 5}, {CoalOre, 1}}},
	{Name: "Diamond Armour", Armor: 3, Cat: CatTools, In: []Ingredient{{DiamondOre, 5}}},
}

// Useful reports whether the recipe would still improve anything for the player.
func (r *Recipe) Useful(p *Player) bool {
	if r.Sword > 0 && p.SwordTier >= r.Sword {
		return false
	}
	if r.Pick > 0 && p.PickTier >= r.Pick {
		return false
	}
	if r.Armor > 0 && p.ArmorTier >= r.Armor {
		return false
	}
	return true
}

func (r *Recipe) CanCraft(p *Player) bool {
	if !r.Useful(p) {
		return false
	}
	for _, in := range r.In {
		if p.Inv[in.Block] < in.N {
			return false
		}
	}
	return true
}

func (r *Recipe) Craft(p *Player) {
	for _, in := range r.In {
		p.Inv[in.Block] -= in.N
	}
	switch {
	case r.Ammo > 0:
		p.Reserve += r.Ammo
	case r.Sword > 0:
		p.SwordTier = r.Sword
	case r.Pick > 0:
		p.PickTier = r.Pick
	case r.Armor > 0:
		p.ArmorTier = r.Armor
	default:
		p.Inv[r.Out] += r.Count
	}
	p.EnsureHeld()
}

func (r *Recipe) needs() string {
	s := ""
	for i, in := range r.In {
		if i > 0 {
			s += " + "
		}
		s += fmt.Sprintf("%d %s", in.N, blocks[in.Block].Name)
	}
	return s
}

var armorColors = [...]rl.Color{rl.Gray, rl.NewColor(150, 95, 55, 255), rl.NewColor(220, 220, 225, 255), rl.NewColor(90, 230, 225, 255)}

// drawArmorIcon draws a chestplate silhouette.
func drawArmorIcon(cx, cy, size float32, tier int) {
	c := armorColors[tier]
	rl.DrawRectangleV(rl.NewVector2(cx-size*0.4, cy-size*0.4), rl.NewVector2(size*0.8, size*0.3), c)
	rl.DrawRectangleV(rl.NewVector2(cx-size*0.3, cy-size*0.1), rl.NewVector2(size*0.6, size*0.5), c)
	rl.DrawRectangleV(rl.NewVector2(cx-size*0.12, cy-size*0.4), rl.NewVector2(size*0.24, size*0.14), rl.NewColor(0, 0, 0, 90))
}

const (
	craftRowH  = 44
	craftW     = 640
	craftTabH  = 30
	craftListY = 50 + craftTabH + 6 // top of the recipe list within the panel
)

// craftRows lists the indices of the recipes shown on the current tab.
func (g *Game) craftRows() []int {
	var rows []int
	for i := range recipes {
		if recipes[i].Cat == g.CraftTab {
			rows = append(rows, i)
		}
	}
	return rows
}

// craftFit returns how many rows of a list of n fit on a screen sh pixels tall.
func craftFit(n int, sh int32) int {
	fit := int((sh - craftListY - 40) / craftRowH)
	if fit < 1 {
		fit = 1
	}
	if n < fit {
		fit = n
	}
	return fit
}

// clampScroll keeps the first visible row of n rows, of which fit are shown, in range.
func clampScroll(scroll, n, fit int) int {
	if scroll > n-fit {
		scroll = n - fit
	}
	if scroll < 0 {
		scroll = 0
	}
	return scroll
}

func (g *Game) craftLayout() (x0, y0, h int32, rows []int, fit int) {
	sw, sh := int32(rl.GetScreenWidth()), int32(rl.GetScreenHeight())
	rows = g.craftRows()
	fit = craftFit(len(rows), sh)
	h = craftListY + int32(fit*craftRowH) + 20
	return sw/2 - craftW/2, sh/2 - h/2, h, rows, fit
}

// updateCrafting handles clicks, the tabs and scrolling on the crafting screen.
func (g *Game) updateCrafting() {
	switch {
	case rl.IsKeyPressed(rl.KeyRight) || rl.IsKeyPressed(rl.KeyTab):
		g.CraftTab = (g.CraftTab + 1) % numCats
		g.CraftScroll = 0
	case rl.IsKeyPressed(rl.KeyLeft):
		g.CraftTab = (g.CraftTab + numCats - 1) % numCats
		g.CraftScroll = 0
	}
	x0, y0, _, rows, fit := g.craftLayout()
	m := rl.GetMousePosition()
	mx, my := int32(m.X), int32(m.Y)
	if wheel := rl.GetMouseWheelMove(); wheel != 0 {
		g.CraftScroll -= int(wheel)
	}
	g.CraftScroll = clampScroll(g.CraftScroll, len(rows), fit)
	g.CraftHover = -1
	if mx < x0 || mx >= x0+craftW {
		return
	}
	if my >= y0+46 && my < y0+46+craftTabH {
		if rl.IsMouseButtonPressed(rl.MouseButtonLeft) {
			tab := Category((mx - x0) * int32(numCats) / craftW)
			if tab != g.CraftTab {
				g.CraftTab = tab
				g.CraftScroll = 0
			}
		}
		return
	}
	row := int((my - y0 - craftListY) / craftRowH)
	if my >= y0+craftListY && row >= 0 && row < fit && g.CraftScroll+row < len(rows) {
		g.CraftHover = rows[g.CraftScroll+row]
	}
	if g.CraftHover >= 0 && rl.IsMouseButtonPressed(rl.MouseButtonLeft) {
		g.craft(g.CraftHover)
	}
}

func (g *Game) craft(i int) {
	r := &recipes[i]
	if !r.CanCraft(g.Player) {
		g.Audio.Play(g.Audio.Click, 0.6)
		return
	}
	r.Craft(g.Player)
	g.Audio.Play(g.Audio.Craft, 0.7)
	g.say("Crafted "+r.Name, 1.2)
}

func (g *Game) drawCrafting() {
	sw, sh := int32(rl.GetScreenWidth()), int32(rl.GetScreenHeight())
	rl.DrawRectangle(0, 0, sw, sh, rl.NewColor(0, 0, 0, 150))
	x0, y0, h, rows, fit := g.craftLayout()
	g.CraftScroll = clampScroll(g.CraftScroll, len(rows), fit)
	rl.DrawRectangle(x0-10, y0-10, craftW+20, h+20, rl.NewColor(198, 198, 198, 255))
	rl.DrawRectangle(x0-6, y0-6, craftW+12, h+12, rl.NewColor(120, 120, 120, 255))
	rl.DrawRectangle(x0, y0, craftW, h, rl.NewColor(160, 160, 160, 255))
	rl.DrawText("CRAFTING", x0+16, y0+12, 28, rl.NewColor(50, 50, 50, 255))
	rl.DrawText("click a recipe    E or ESC to close", x0+craftW-330, y0+20, 16, rl.NewColor(70, 70, 70, 255))
	// Tabs.
	tabW := craftW / int32(numCats)
	for c := Category(0); c < numCats; c++ {
		tx := x0 + int32(c)*tabW
		bg := rl.NewColor(130, 130, 130, 255)
		fg := rl.NewColor(60, 60, 60, 255)
		if c == g.CraftTab {
			bg = rl.NewColor(210, 210, 190, 255)
			fg = rl.NewColor(30, 30, 30, 255)
		}
		rl.DrawRectangle(tx+4, y0+46, tabW-8, craftTabH, bg)
		tw := rl.MeasureText(catNames[c], 18)
		rl.DrawText(catNames[c], tx+tabW/2-tw/2, y0+46+6, 18, fg)
	}
	p := g.Player
	for k := 0; k < fit; k++ {
		i := rows[g.CraftScroll+k]
		r := &recipes[i]
		y := y0 + craftListY + int32(k*craftRowH)
		bg := rl.NewColor(140, 140, 140, 255)
		if i == g.CraftHover {
			bg = rl.NewColor(210, 210, 190, 255)
		}
		rl.DrawRectangle(x0+8, y, craftW-16, craftRowH-4, bg)
		name := rl.NewColor(40, 40, 40, 255)
		need := rl.NewColor(80, 80, 80, 255)
		switch {
		case !r.Useful(p):
			name = rl.NewColor(110, 110, 110, 255)
			need = rl.NewColor(120, 120, 120, 255)
		case r.CanCraft(p):
			name = rl.NewColor(20, 90, 20, 255)
		default:
			name = rl.NewColor(120, 40, 40, 255)
		}
		// Result icon.
		ix, iy := x0+16, y+6
		switch {
		case r.Ammo > 0:
			rl.DrawRectangle(ix+6, iy+8, 20, 16, rl.NewColor(240, 190, 50, 255))
		case r.Sword > 0:
			drawSwordIcon(float32(ix+16), float32(iy+16), 30, r.Sword)
		case r.Pick > 0:
			drawPickIcon(float32(ix+16), float32(iy+16), 30, r.Pick)
		case r.Armor > 0:
			drawArmorIcon(float32(ix+16), float32(iy+16), 28, r.Armor)
		default:
			g.drawBlockIcon(r.Out, ix, iy, 32)
		}
		label := r.Name
		if !r.Useful(p) {
			label += "  (owned)"
		}
		rl.DrawText(label, x0+60, y+6, 20, name)
		rl.DrawText(r.needs(), x0+60, y+26, 14, need)
	}
	// Scrollbar when the tab holds more than fits.
	if len(rows) > fit {
		trackY, trackH := y0+craftListY, int32(fit*craftRowH)-4
		rl.DrawRectangle(x0+craftW-6, trackY, 4, trackH, rl.NewColor(120, 120, 120, 255))
		thumbH := trackH * int32(fit) / int32(len(rows))
		thumbY := trackY + (trackH-thumbH)*int32(g.CraftScroll)/int32(len(rows)-fit)
		rl.DrawRectangle(x0+craftW-6, thumbY, 4, thumbH, rl.NewColor(60, 60, 60, 255))
		rl.DrawText("scroll for more", x0+craftW-130, y0+h-18, 14, rl.NewColor(70, 70, 70, 255))
	}
	// Inventory summary on the right.
	invX := x0 + craftW + 30
	if invX+200 < sw {
		rl.DrawRectangle(invX-10, y0-10, 220, h+20, rl.NewColor(160, 160, 160, 255))
		rl.DrawText("INVENTORY", invX, y0+8, 20, rl.NewColor(50, 50, 50, 255))
		rl.DrawText(fmt.Sprintf("Ammo %d", p.Ammo+p.Reserve), invX, y0+34, 16, rl.NewColor(60, 60, 60, 255))
		rl.DrawText(fmt.Sprintf("%s sword", tierNames[p.SwordTier]), invX, y0+52, 16, rl.NewColor(60, 60, 60, 255))
		rl.DrawText(fmt.Sprintf("%s pickaxe", tierNames[p.PickTier]), invX, y0+70, 16, rl.NewColor(60, 60, 60, 255))
		armour := "No armour"
		if p.ArmorTier > 0 {
			armour = armorNames[p.ArmorTier] + " armour"
		}
		rl.DrawText(armour, invX, y0+88, 16, rl.NewColor(60, 60, 60, 255))
		yy := y0 + 114
		for b := Block(1); b < numBlocks; b++ {
			if p.Inv[b] == 0 {
				continue
			}
			g.drawBlockIcon(b, invX, yy, 22)
			rl.DrawText(fmt.Sprintf("%d  %s", p.Inv[b], blocks[b].Name), invX+30, yy+3, 16, rl.NewColor(40, 40, 40, 255))
			yy += 26
			if yy > y0+h-20 {
				break
			}
		}
	}
}
