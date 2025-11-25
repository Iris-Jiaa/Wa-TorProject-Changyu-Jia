/**
 * @file main.go
 * @brief Wa-Tor simulation implementation
 * @author Changyu Jia
 * @date 2025
 *
 * This file implements the Wa-Tor ecological simulation as described in
 * A.K. Dewdney's Scientific American article "Sharks and Fish wage an
 * ecological war on the toroidal planet of Wa-Tor".
 */

package main

import (
	"flag"
	"fmt"
	"image/color"
	"log"
	"math/rand"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/hajimehoshi/ebiten/v2"
)

// Command line parameters for the simulation
var (
	numSharks    = flag.Int("NumShark", 6000, "Starting population of sharks")
	numFish      = flag.Int("NumFish", 70000, "Starting population of fish")
	fishBreed    = flag.Int("FishBreed", 6, "Number of time units that pass before a fish can reproduce")
	sharkBreed   = flag.Int("SharkBreed", 16, "Number of time units that must pass before a shark can reproduce")
	starve       = flag.Int("Starve", 4, "Period of time a shark can go without food before dying")
	gridSize     = flag.Int("GridSize", 300, "Dimensions of world")
	threads      = flag.Int("Threads", runtime.NumCPU(), "Number of concurrent goroutines to use")
	benchmark    = flag.Bool("benchmark", false, "Run in benchmark mode (no graphics)")
	steps        = flag.Int("steps", 4000, "Number of steps for benchmark")
	windowWidth  = flag.Int("width", 1400, "Window width")
	windowHeight = flag.Int("height", 900, "Window height")
)

const (
	pixSize = 2 ///< Pixel size for rendering each cell
)

/**
 * @brief Type of creature in the simulation
 */
type CreatureType int

const (
	Fish  CreatureType = iota ///< Fish creature type
	Shark                     ///< Shark creature type
)

/**
 * @brief Represents an individual creature in the Wa-Tor world
 *
 * Each creature has a position, age, energy (for sharks), and type.
 * The simulation evolves based on the rules governing these creatures.
 */
type Creature struct {
	x, y         int          ///< Grid coordinates of the creature
	age          int          ///< Age in chronons (time steps)
	energy       int          ///< Energy level (sharks only)
	creatureType CreatureType ///< Type of creature (Fish or Shark)
}

/**
 * @brief The main world grid representing the Wa-Tor planet
 *
 * The world is a toroidal grid (wraps around edges) containing creatures.
 * Uses double buffering (grid and nextGrid) for concurrent updates.
 */
type World struct {
	width, height  int         ///< Dimensions of the world grid
	screenWidth    int         ///< Screen width in pixels
	screenHeight   int         ///< Screen height in pixels
	grid           []*Creature ///< Current state grid
	nextGrid       []*Creature ///< Next state grid (double buffering)
	goroutineCount int         ///< Number of goroutines for parallel processing
}

/**
 * @brief Creates a new World instance
 * @param width Grid width
 * @param height Grid height
 * @param goroutineCount Number of goroutines for parallel processing
 * @return Pointer to the newly created World
 */
func NewWorld(width, height, goroutineCount int) *World {
	w := &World{
		width:          width,
		height:         height,
		screenWidth:    width * pixSize,
		screenHeight:   height * pixSize,
		grid:           make([]*Creature, width*height),
		nextGrid:       make([]*Creature, width*height),
		goroutineCount: goroutineCount,
	}
	return w
}

/**
 * @brief Converts grid coordinates to array index with toroidal wrapping
 * @param x X coordinate
 * @param y Y coordinate
 * @return Array index for the given coordinates
 */
func (w *World) idx(x, y int) int {
	if x < 0 {
		x += w.width
	} else if x >= w.width {
		x -= w.width
	}
	if y < 0 {
		y += w.height
	} else if y >= w.height {
		y -= w.height
	}
	return x + y*w.width
}

/**
 * @brief Initializes the world with starting populations of sharks and fish
 * @param numSharks Initial number of sharks
 * @param numFish Initial number of fish
 *
 * Randomly places creatures on the grid, ensuring no two creatures occupy
 * the same cell. If total creatures exceed grid capacity, adjusts populations.
 */
func (w *World) InitPopulation(numSharks, numFish int) {
	for i := range w.grid {
		w.grid[i] = nil
	}

	count := 0
	limit := w.width * w.height
	if numFish+numSharks > limit {
		numFish = limit / 2
		numSharks = limit / 2
	}

	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	// Place fish randomly
	for i := 0; i < numFish; i++ {
		for {
			x, y := r.Intn(w.width), r.Intn(w.height)
			idx := w.idx(x, y)
			if w.grid[idx] == nil {
				w.grid[idx] = &Creature{x: x, y: y, age: r.Intn(*fishBreed), creatureType: Fish}
				count++
				break
			}
		}
	}

	// Place sharks randomly
	for i := 0; i < numSharks; i++ {
		for {
			x, y := r.Intn(w.width), r.Intn(w.height)
			idx := w.idx(x, y)
			if w.grid[idx] == nil {
				w.grid[idx] = &Creature{x: x, y: y, age: r.Intn(*sharkBreed), energy: *starve, creatureType: Shark}
				count++
				break
			}
		}
	}
}

/**
 * @brief Advances the simulation by one time step (chronon)
 *
 * Processes the world in parallel using multiple goroutines.
 * Implements double buffering to avoid race conditions.
 */
func (w *World) Update() {
	var wg sync.WaitGroup

	// Divide grid into horizontal slices for parallel processing
	rowsPerGoroutine := (w.height + w.goroutineCount - 1) / w.goroutineCount
	if rowsPerGoroutine < 1 {
		rowsPerGoroutine = 1
	}

	// Launch goroutines for each slice
	for i := 0; i < w.goroutineCount; i++ {
		startY := i * rowsPerGoroutine
		if startY >= w.height {
			break
		}
		endY := startY + rowsPerGoroutine
		if endY > w.height {
			endY = w.height
		}

		wg.Add(1)
		go func(id, sY, eY int) {
			defer wg.Done()
			seed := time.Now().UnixNano() + int64(id*100)
			rng := rand.New(rand.NewSource(seed))
			w.processSlice(sY, eY, rng)
		}(i, startY, endY)
	}

	wg.Wait()

	// Swap buffers for next frame
	w.grid, w.nextGrid = w.nextGrid, w.grid

	// Clear the nextGrid for future use
	for i := range w.nextGrid {
		w.nextGrid[i] = nil
	}
}

/**
 * @brief Processes a slice of the world grid
 * @param startY Starting Y coordinate (inclusive)
 * @param endY Ending Y coordinate (exclusive)
 * @param rng Random number generator for the goroutine
 *
 * Applies Wa-Tor rules to each creature in the slice:
 * - Fish move randomly and reproduce after FishBreed chronons
 * - Sharks hunt fish, lose energy, and reproduce after SharkBreed chronons
 * - Sharks die when energy reaches zero
 */
func (w *World) processSlice(startY, endY int, rng *rand.Rand) {
	// Directions: north, east, south, west
	dirs := [4][2]int{{0, -1}, {1, 0}, {0, 1}, {-1, 0}}

	for y := startY; y < endY; y++ {
		for x := 0; x < w.width; x++ {
			idx := w.idx(x, y)
			c := w.grid[idx]

			if c == nil {
				continue
			}

			// Create a copy of the creature for the next state
			nextC := *c
			nextC.age++

			moved := false

			if nextC.creatureType == Fish {
				// Fish behavior: move randomly to empty adjacent cell
				perm := rng.Perm(4)
				for _, i := range perm {
					dx, dy := dirs[i][0], dirs[i][1]
					nx, ny := x+dx, y+dy
					if nx < 0 {
						nx += w.width
					} else if nx >= w.width {
						nx -= w.width
					}
					if ny < 0 {
						ny += w.height
					} else if ny >= w.height {
						ny -= w.height
					}

					nIdx := w.idx(nx, ny)

					if w.grid[nIdx] == nil {
						// Use atomic operation for thread-safe writing to nextGrid
						if atomic.CompareAndSwapPointer(
							(*unsafe.Pointer)(unsafe.Pointer(&w.nextGrid[nIdx])),
							nil,
							unsafe.Pointer(&nextC)) {

							moved = true

							// Reproduction logic
							if c.age >= *fishBreed {
								nextC.age = 0
								baby := &Creature{x: x, y: y, age: 0, creatureType: Fish}
								atomic.CompareAndSwapPointer(
									(*unsafe.Pointer)(unsafe.Pointer(&w.nextGrid[idx])),
									nil,
									unsafe.Pointer(baby))
							}
							break
						}
					}
				}

			} else {
				// Shark behavior: lose energy each turn
				nextC.energy--
				if nextC.energy <= 0 {
					// Shark dies from starvation
					continue
				}

				perm := rng.Perm(4)
				ate := false

				// Priority 1: Hunt fish
				for _, i := range perm {
					dx, dy := dirs[i][0], dirs[i][1]
					nx, ny := x+dx, y+dy
					if nx < 0 {
						nx += w.width
					} else if nx >= w.width {
						nx -= w.width
					}
					if ny < 0 {
						ny += w.height
					} else if ny >= w.height {
						ny -= w.height
					}
					nIdx := w.idx(nx, ny)

					// Check if target cell has a fish
					target := w.grid[nIdx]
					if target != nil && target.creatureType == Fish {
						// Eat the fish and gain energy
						nextC.energy += *starve

						// Move to the fish's location
						if atomic.CompareAndSwapPointer(
							(*unsafe.Pointer)(unsafe.Pointer(&w.nextGrid[nIdx])),
							nil,
							unsafe.Pointer(&nextC)) {

							moved = true
							ate = true

							// Reproduction logic
							if c.age >= *sharkBreed {
								nextC.age = 0
								baby := &Creature{x: x, y: y, age: 0, energy: *starve, creatureType: Shark}
								atomic.CompareAndSwapPointer(
									(*unsafe.Pointer)(unsafe.Pointer(&w.nextGrid[idx])),
									nil,
									unsafe.Pointer(baby))
							}
							break
						}
					}
				}

				// Priority 2: Move like fish if no fish to eat
				if !ate {
					perm2 := rng.Perm(4)
					for _, i := range perm2 {
						dx, dy := dirs[i][0], dirs[i][1]
						nx, ny := x+dx, y+dy
						if nx < 0 {
							nx += w.width
						} else if nx >= w.width {
							nx -= w.width
						}
						if ny < 0 {
							ny += w.height
						} else if ny >= w.height {
							ny -= w.height
						}
						nIdx := w.idx(nx, ny)

						if w.grid[nIdx] == nil {
							if atomic.CompareAndSwapPointer(
								(*unsafe.Pointer)(unsafe.Pointer(&w.nextGrid[nIdx])),
								nil,
								unsafe.Pointer(&nextC)) {

								moved = true
								if c.age >= *sharkBreed {
									nextC.age = 0
									baby := &Creature{x: x, y: y, age: 0, energy: *starve, creatureType: Shark}
									atomic.CompareAndSwapPointer(
										(*unsafe.Pointer)(unsafe.Pointer(&w.nextGrid[idx])),
										nil,
										unsafe.Pointer(baby))
								}
								break
							}
						}
					}
				}
			}

			// If creature couldn't move, stay in current position
			if !moved {
				atomic.CompareAndSwapPointer(
					(*unsafe.Pointer)(unsafe.Pointer(&w.nextGrid[idx])),
					nil,
					unsafe.Pointer(&nextC))
			}
		}
	}
}

/**
 * @brief Runs the simulation in benchmark mode without graphics
 *
 * Measures performance with different numbers of threads (1, 2, 4, 8)
 * and generates a speedup report comparing the results.
 */
func runBenchmark() {
	coreConfigs := []int{1, 2, 4, 8}
	results := make(map[int]time.Duration, 3)

	fmt.Println("=== Wa-Tor Simulation Benchmark ===")
	fmt.Printf("Grid Size: %d, Sharks: %d, Fish: %d, Steps: %d\n",
		*gridSize, *numSharks, *numFish, *steps)
	fmt.Println("-----------------------------------")

	for _, c := range coreConfigs {
		world := NewWorld(*gridSize, *gridSize, c)
		world.InitPopulation(*numSharks, *numFish)

		// Warm-up run
		for i := 0; i < 50; i++ {
			world.Update()
		}
		runtime.GC()

		// Benchmark run
		start := time.Now()
		for i := 0; i < *steps; i++ {
			world.Update()
		}
		elapsed := time.Since(start)
		results[c] = elapsed

		fmt.Printf("Threads: %d, Time: %v\n", c, elapsed)
	}

	generateSpeedupReport(results)
}

/**
 * @brief Generates a speedup report comparing performance with different thread counts
 * @param results Map of thread counts to execution times
 */
func generateSpeedupReport(results map[int]time.Duration) {
	baseTime := results[1].Seconds()
	fmt.Println("\n=== Speedup Report ===")

	for _, c := range []int{1, 2, 4, 8} {
		if t, exists := results[c]; exists {
			ts := t.Seconds()
			speedup := baseTime / ts
			efficiency := (speedup / float64(c)) * 100
			fmt.Printf("%d Threads: %.4fs (Speedup: %.2fx, Efficiency: %.1f%%)\n",
				c, ts, speedup, efficiency)
		}
	}
}

/**
 * @brief Game structure for graphical rendering using Ebiten
 */
type Game struct {
	world    *World        ///< The Wa-Tor world simulation
	fishImg  *ebiten.Image ///< Image for rendering fish
	sharkImg *ebiten.Image ///< Image for rendering sharks
}

/**
 * @brief Creates a new Game instance
 * @return Pointer to the newly created Game
 */
func NewGame() *Game {
	w := NewWorld(*windowWidth/pixSize, *windowHeight/pixSize, *threads)
	w.InitPopulation(*numSharks, *numFish)

	fishImg := ebiten.NewImage(pixSize, pixSize)
	fishImg.Fill(color.RGBA{80, 212, 80, 255})
	sharkImg := ebiten.NewImage(pixSize, pixSize)
	sharkImg.Fill(color.RGBA{100, 100, 252, 255})

	return &Game{world: w, fishImg: fishImg, sharkImg: sharkImg}
}

/**
 * @brief Updates the game state (called each frame)
 * @return Error if any occurred
 */
func (g *Game) Update() error {
	g.world.Update()
	return nil
}

/**
 * @brief Renders the current state of the world
 * @param screen The target image to render to
 */
func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(color.Black)
	op := &ebiten.DrawImageOptions{}
	for i, c := range g.world.grid {
		if c != nil {
			op.GeoM.Reset()
			x := i % g.world.width
			y := i / g.world.width
			op.GeoM.Translate(float64(x*pixSize), float64(y*pixSize))

			if c.creatureType == Fish {
				screen.DrawImage(g.fishImg, op)
			} else {
				screen.DrawImage(g.sharkImg, op)
			}
		}
	}
}

/**
 * @brief Returns the game's logical screen size
 * @param outsideWidth Window width
 * @param outsideHeight Window height
 * @return Logical width and height
 */
func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return g.world.screenWidth, g.world.screenHeight
}

/**
 * @brief Main entry point of the application
 *
 * Parses command line arguments and starts either benchmark mode
 * or graphical simulation mode based on the -benchmark flag.
 */
func main() {
	flag.Parse()
	rand.Seed(time.Now().UnixNano())

	if *benchmark {
		runBenchmark()
		return
	}

	ebiten.SetWindowSize(*windowWidth, *windowHeight)
	ebiten.SetWindowTitle(fmt.Sprintf("Wa-Tor (Threads: %d, Size: %dx%d)", *threads, *windowWidth, *windowHeight))

	ebiten.SetVsyncEnabled(false)

	if err := ebiten.RunGame(NewGame()); err != nil {
		log.Fatal(err)
	}
}
