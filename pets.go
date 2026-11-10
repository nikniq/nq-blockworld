package main

import (
	rl "github.com/gen2brain/raylib-go/raylib"
)

// Pets: feed an ordinary animal its favourite food twice and it becomes a
// pet. Pets follow the player (teleporting to catch up), never run from them,
// and can be told to sit. Wolves, cats and horses have their own taming code.

// petFoods lists what wins each animal over.
var petFoods = map[AnimalKind][]Block{
	AnimalPig:      {Carrot, Beetroot, Apple},
	AnimalCow:      {WheatItem},
	AnimalSheep:    {WheatItem},
	AnimalKangaroo: {Carrot, Apple},
	AnimalEmu:      {PumpkinSeeds, MelonSeeds, WheatItem},
	AnimalKoala:    {EucLeaves},
	AnimalWombat:   {Carrot, Beetroot},
	AnimalPlatypus: {Fish},
	AnimalCompy:    {Meat, Fish},
}

// petFoodList names the foods an animal likes, for hints.
func petFoodList(k AnimalKind) string {
	s := ""
	for i, b := range petFoods[k] {
		switch {
		case i == 0:
		case i == len(petFoods[k])-1:
			s += " or "
		default:
			s += ", "
		}
		s += blocks[b].Name
	}
	return s
}

// likes reports whether the held item is something the animal would eat.
func (a *Animal) likes(held Item) bool {
	if held.Kind != ItemFood && held.Kind != ItemBlock {
		return false
	}
	for _, b := range petFoods[a.Kind] {
		if b == held.Block {
			return true
		}
	}
	return false
}

// isPet reports whether the animal follows the generic pet rules.
func (a *Animal) isPet() bool {
	_, ok := petFoods[a.Kind]
	return ok && a.Tamed
}

// tamePet handles a right click on one of the ordinary animals.
func (g *Game) tamePet(a *Animal) {
	p := g.Player
	name := a.Spec.Name
	if a.Tamed {
		if a.likes(p.Held) && a.HP < a.Spec.HP {
			p.Inv[p.Held.Block]--
			p.EnsureHeld()
			a.HP = a.Spec.HP
			g.say("Your "+name+" munches happily", 1.2)
			g.Audio.Play(g.Audio.Eat, 0.6)
			return
		}
		a.Sitting = !a.Sitting
		if a.Sitting {
			g.say("Your "+name+" waits here", 1.2)
		} else {
			g.say("Your "+name+" follows you", 1.2)
		}
		g.Audio.Play(g.Audio.Click, 0.5)
		return
	}
	if !a.likes(p.Held) || p.Inv[p.Held.Block] == 0 {
		g.say("The "+name+" looks at you. It would like "+petFoodList(a.Kind)+".", 2)
		return
	}
	p.Inv[p.Held.Block]--
	p.EnsureHeld()
	a.Flee = 0
	a.HP = a.Spec.HP
	g.burst(rl.Vector3Add(a.Pos, rl.NewVector3(0, a.Spec.Height*0.8, 0)), rl.NewColor(255, 120, 150, 255), 8)
	g.Audio.Play(g.Audio.Eat, 0.6)
	a.Visit++
	if a.Visit >= 2 {
		a.Tamed = true
		a.Sitting = false
		g.say("The "+name+" is your pet. It will follow you; right click to make it wait.", 3)
		g.Audio.Play(g.Audio.Clear, 0.7)
		g.unlock(AchPet)
	} else {
		g.say("The "+name+" nuzzles your hand. One more should do it.", 1.8)
	}
}

// followOwner steers a pet after the player: it stops when close, walks when
// a few blocks behind and teleports when left far behind. Sitting pets stay.
func (g *Game) followOwner(a *Animal) {
	a.Flee = 0
	if a.Sitting {
		a.Walking = false
		a.WanderT = 1
		return
	}
	p := g.Player
	d := WrapDelta(p.Pos, a.Pos)
	d.Y = 0
	dist := rl.Vector3Length(d)
	switch {
	case dist > 24:
		a.Pos = rl.Vector3Add(p.Pos, rl.NewVector3(-1.2, 0.2, 1.2))
	case dist > 3.5:
		a.Heading = rl.Vector3Scale(d, 1/dist)
		a.Walking = true
		a.WanderT = 0.3
	case dist < 2:
		a.Walking = false
		a.WanderT = 0.6
	}
}

// drawCollar marks a pet with a collar around the neck.
func drawCollar(a *Animal, col rl.Color) {
	fwd := a.Heading
	if rl.Vector3Length(fwd) < 0.01 {
		fwd = rl.NewVector3(0, 0, 1)
	}
	h := a.Spec.Height
	c := rl.Vector3Add(rl.NewVector3(a.Pos.X, a.Pos.Y+h*0.7, a.Pos.Z), rl.Vector3Scale(fwd, a.Spec.Radius*1.1))
	w := a.Spec.Radius * 1.3
	rl.DrawCubeV(c, rl.NewVector3(w, 0.08, w), mul(col, a.Lum))
}
