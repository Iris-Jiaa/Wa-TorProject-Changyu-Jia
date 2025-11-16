package main

import (
	"flag"
	"image/color"
	"log"
	"math/rand"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

var (
	numSharks  = flag.Int("NumShark", 40, "Starting population of sharks")
	numFish    = flag.Int("NumFish", 600, "Starting population of fish")
	fishBreed  = flag.Int("FishBreed", 8, "Number of time units that pass before a fish can reproduce")
	sharkBreed = flag.Int("SharkBreed", 16, "Number of time units that must pass before a shark can reproduce")
	starve     = flag.Int("Starve", 6, "Period of time a shark can go without food before dying")
	gridSize   = flag.Int("GridSize", 100, "Dimensions of world")
	threads    = flag.Int("Threads", 1, "Number of threads to use (TODO: parallelize update)")
)

const (
	pixSize = 5
)

type CreatureType int

const (
	Water CreatureType = iota
	Fish
	Shark
)

type Creature struct {
	x, y         int
	age          int
	energy       int
	creatureType CreatureType
	moved        bool
}

type World struct {
	width, height int
	screenWidth   int
	screenHeight  int

	grid      []CreatureType
	creatures []*Creature
	rng       *rand.Rand
}

func NewWorld(size int) *World {
	w := &World{
		width:        size,
		height:       size,
		screenWidth:  size * pixSize,
		screenHeight: size * pixSize,
		grid:         make([]CreatureType, size*size),
		creatures:    make([]*Creature, 0),
		rng:          rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	return w
}

func (w *World) idx(x, y int) int {
	return x + y*w.width
}

func (w *World) InitPopulation(numSharks, numFish int) {
	total := numSharks + numFish
	if total <= 0 {
		total = 1
	}
	w.creatures = make([]*Creature, 0, total)

	for i := 0; i < numFish; i++ {
		x, y := w.getRandomEmptyCell()
		if x != -1 {
			fish := &Creature{x: x, y: y, age: 0, creatureType: Fish}
			w.creatures = append(w.creatures, fish)
			w.grid[w.idx(x, y)] = Fish
		}
	}
	for i := 0; i < numSharks; i++ {
		x, y := w.getRandomEmptyCell()
		if x != -1 {
			shark := &Creature{x: x, y: y, age: 0, energy: *starve, creatureType: Shark}
			w.creatures = append(w.creatures, shark)
			w.grid[w.idx(x, y)] = Shark
		}
	}
}

// getRandomEmptyCell finds a random empty cell
func (w *World) getRandomEmptyCell() (int, int) {
	attempts := 0
	maxAttempts := w.width * w.height * 2
	for attempts < maxAttempts {
		x := w.rng.Intn(w.width)
		y := w.rng.Intn(w.height)
		if w.grid[w.idx(x, y)] == Water {
			return x, y
		}
		attempts++
	}
	return -1, -1
}

// compactCreatures removes entries whose creatureType == Water or dead sharks.
// This reduces iteration overhead by keeping creatures slice small.
func (w *World) compactCreatures() {
	out := w.creatures[:0]
	for _, c := range w.creatures {
		if c == nil {
			continue
		}
		if c.creatureType == Water {
			// ensure grid cell is water if creature marked as water
			if w.grid[w.idx(c.x, c.y)] == c.creatureType {
				w.grid[w.idx(c.x, c.y)] = Water
			}
			continue
		}
		if c.creatureType == Shark && c.energy <= 0 {
			// remove shark from grid
			if w.grid[w.idx(c.x, c.y)] == Shark {
				w.grid[w.idx(c.x, c.y)] = Water
			}
			continue
		}
		out = append(out, c)
	}
	w.creatures = out
}

// Update advances the simulation by one tick
func (w *World) Update() {
	// 1) Remove already-dead or eaten creatures to reduce work.
	w.compactCreatures()

	// 2) local copies of global parameters to avoid repeated pointer loads
	fb := *fishBreed
	sb := *sharkBreed
	s := *starve

	// 3) reset moved flags
	for _, c := range w.creatures {
		c.moved = false
	}

	// Estimate new creatures count to minimize allocations
	newCreatures := make([]*Creature, 0, len(w.creatures)/4+10)

	// Process each creature
	for _, c := range w.creatures {
		// skip any creature that might have been turned to Water earlier (safety)
		if c.moved || c.creatureType == Water || (c.creatureType == Shark && c.energy <= 0) {
			continue
		}
		switch c.creatureType {
		case Shark:
			w.updateShark(c, &newCreatures, sb, s)
		case Fish:
			w.updateFish(c, &newCreatures, fb)
		}
	}

	// Append newborns
	if len(newCreatures) > 0 {
		w.creatures = append(w.creatures, newCreatures...)
	}

	// Final cleanup
	w.compactCreatures()
}

// helper: neighbor directions (N,E,S,W)
var directions = [][2]int{{0, -1}, {1, 0}, {0, 1}, {-1, 0}}

// findNeighborsByType finds neighbors of a specific type
func (w *World) findNeighborsByType(x, y int, creatureType CreatureType) [][2]int {
	neighbors := make([][2]int, 0, 4)
	for _, d := range directions {
		nx := (x + d[0] + w.width) % w.width
		ny := (y + d[1] + w.height) % w.height
		if w.grid[w.idx(nx, ny)] == creatureType {
			neighbors = append(neighbors, [2]int{nx, ny})
		}
	}
	return neighbors
}

// updateShark updates a shark's state
func (w *World) updateShark(c *Creature, newCreatures *[]*Creature, sharkBreedParam, starveParam int) {
	c.age++
	c.energy--

	// Shark dies if energy reaches zero
	if c.energy <= 0 {
		// mark as dead; will be compacted later
		c.creatureType = Water
		w.grid[w.idx(c.x, c.y)] = Water
		return
	}

	oldX, oldY := c.x, c.y
	newX, newY := oldX, oldY

	// Look for fish to eat first
	fishNeighbors := w.findNeighborsByType(c.x, c.y, Fish)
	if len(fishNeighbors) > 0 {
		// Eat a fish
		target := fishNeighbors[w.rng.Intn(len(fishNeighbors))]
		newX, newY = target[0], target[1]

		// Remove fish from grid and mark it Water in creatures list by scanning creatures.
		// (We avoid a full grid scan by checking list — it's acceptable for moderate sizes.)
		for _, fish := range w.creatures {
			if fish.x == newX && fish.y == newY && fish.creatureType == Fish {
				fish.creatureType = Water // mark eaten fish
				break
			}
		}
		c.energy += starveParam
	} else {
		// Move to empty cell if no fish found
		emptyNeighbors := w.findNeighborsByType(c.x, c.y, Water)
		if len(emptyNeighbors) > 0 {
			target := emptyNeighbors[w.rng.Intn(len(emptyNeighbors))]
			newX, newY = target[0], target[1]
		}
	}

	// Move shark if position changed
	if newX != oldX || newY != oldY {
		w.grid[w.idx(oldX, oldY)] = Water
		c.x, c.y = newX, newY
		w.grid[w.idx(newX, newY)] = Shark
	}
	c.moved = true

	// Reproduction
	if c.age >= sharkBreedParam {
		c.age = 0
		// Create new shark in old position if it's empty
		if w.grid[w.idx(oldX, oldY)] == Water {
			newShark := &Creature{x: oldX, y: oldY, age: 0, energy: starveParam, creatureType: Shark}
			w.grid[w.idx(oldX, oldY)] = Shark
			*newCreatures = append(*newCreatures, newShark)
		}
	}
}

// updateFish updates a fish's state
func (w *World) updateFish(c *Creature, newCreatures *[]*Creature, fishBreedParam int) {
	c.age++

	oldX, oldY := c.x, c.y
	newX, newY := oldX, oldY

	emptyNeighbors := w.findNeighborsByType(c.x, c.y, Water)
	if len(emptyNeighbors) > 0 {
		target := emptyNeighbors[w.rng.Intn(len(emptyNeighbors))]
		newX, newY = target[0], target[1]
	}

	// Move fish if position changed
	if newX != oldX || newY != oldY {
		w.grid[w.idx(oldX, oldY)] = Water
		c.x, c.y = newX, newY
		w.grid[w.idx(newX, newY)] = Fish
	}
	c.moved = true

	// Reproduction
	if c.age >= fishBreedParam {
		c.age = 0
		if w.grid[w.idx(oldX, oldY)] == Water {
			newFish := &Creature{x: oldX, y: oldY, age: 0, creatureType: Fish}
			w.grid[w.idx(oldX, oldY)] = Fish
			*newCreatures = append(*newCreatures, newFish)
		}
	}
}

/* Game implements ebiten.Game */
type Game struct {
	world    *World
	fishImg  *ebiten.Image
	sharkImg *ebiten.Image
}

// NewGame creates a new Game
func NewGame() *Game {
	w := NewWorld(*gridSize)
	w.InitPopulation(*numSharks, *numFish)

	// create small sprites for fish and shark to speed up drawing
	fishImg := ebiten.NewImage(pixSize, pixSize)
	fishImg.Fill(color.RGBA{80, 212, 80, 255})
	sharkImg := ebiten.NewImage(pixSize, pixSize)
	sharkImg.Fill(color.RGBA{100, 100, 252, 255})

	return &Game{world: w, fishImg: fishImg, sharkImg: sharkImg}
}

// Update proceeds the game state.
func (g *Game) Update() error {
	g.world.Update()
	return nil
}

// Draw draws the game screen.
func (g *Game) Draw(screen *ebiten.Image) {
	// Fill background once
	screen.Fill(color.Black)

	op := &ebiten.DrawImageOptions{}
	for _, c := range g.world.creatures {
		if c.creatureType == Fish {
			op.GeoM.Reset()
			op.GeoM.Translate(float64(c.x*pixSize), float64(c.y*pixSize))
			screen.DrawImage(g.fishImg, op)
		} else if c.creatureType == Shark {
			op.GeoM.Reset()
			op.GeoM.Translate(float64(c.x*pixSize), float64(c.y*pixSize))
			screen.DrawImage(g.sharkImg, op)
		}
	}
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return g.world.screenWidth, g.world.screenHeight
}

func main() {
	flag.Parse()

	ebiten.SetWindowSize((*gridSize)*pixSize, (*gridSize)*pixSize)
	ebiten.SetWindowTitle("Wator Simulation (optimized)")

	if err := ebiten.RunGame(NewGame()); err != nil {
		log.Fatal(err)
	}
}
