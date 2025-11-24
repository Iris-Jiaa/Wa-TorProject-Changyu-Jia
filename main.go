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

var (
	numSharks    = flag.Int("NumShark", 300, "Starting population of sharks")
	numFish      = flag.Int("NumFish", 6000, "Starting population of fish")
	fishBreed    = flag.Int("FishBreed", 6, "Number of time units that pass before a fish can reproduce")
	sharkBreed   = flag.Int("SharkBreed", 16, "Number of time units that must pass before a shark can reproduce")
	starve       = flag.Int("Starve", 4, "Period of time a shark can go without food before dying")
	gridSize     = flag.Int("GridSize", 100, "Dimensions of world")
	threads      = flag.Int("Threads", runtime.NumCPU(), "Number of concurrent goroutines to use")
	benchmark    = flag.Bool("benchmark", false, "Run in benchmark mode (no graphics)")
	steps        = flag.Int("steps", 20000, "Number of steps for benchmark")
	windowWidth  = flag.Int("width", 1400, "Window width")
	windowHeight = flag.Int("height", 900, "Window height")
)

const (
	pixSize = 2
)

type CreatureType int

const (
	Fish CreatureType = iota
	Shark
)

type Creature struct {
	x, y         int
	age          int
	energy       int
	creatureType CreatureType
}

type World struct {
	width, height int
	screenWidth   int
	screenHeight  int
	grid          []*Creature
	nextGrid      []*Creature

	goroutineCount int
}

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

func (w *World) Update() {
	var wg sync.WaitGroup

	rowsPerGoroutine := (w.height + w.goroutineCount - 1) / w.goroutineCount
	if rowsPerGoroutine < 1 {
		rowsPerGoroutine = 1
	}

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

	w.grid, w.nextGrid = w.nextGrid, w.grid

	for i := range w.nextGrid {
		w.nextGrid[i] = nil
	}
}

func (w *World) processSlice(startY, endY int, rng *rand.Rand) {
	dirs := [4][2]int{{0, -1}, {1, 0}, {0, 1}, {-1, 0}}

	for y := startY; y < endY; y++ {
		for x := 0; x < w.width; x++ {
			idx := w.idx(x, y)
			c := w.grid[idx]

			if c == nil {
				continue
			}

			nextC := *c
			nextC.age++

			moved := false

			if nextC.creatureType == Fish {
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
						// 尝试原子写入 nextGrid (并发安全)
						// 这里的 atomic.CompareAndSwapPointer 相当于：
						// if nextGrid[nIdx] == nil { nextGrid[nIdx] = &nextC; return true } else { return false }
						if atomic.CompareAndSwapPointer(
							(*unsafe.Pointer)(unsafe.Pointer(&w.nextGrid[nIdx])),
							nil,
							unsafe.Pointer(&nextC)) {

							moved = true

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
				nextC.energy--
				if nextC.energy <= 0 {
					continue
				}

				perm := rng.Perm(4)
				ate := false
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

					// 检查 grid 中该位置是否有鱼
					target := w.grid[nIdx]
					if target != nil && target.creatureType == Fish {
						// 吃鱼！
						nextC.energy += *starve // 恢复能量
						// 尝试移动到鱼的位置
						if atomic.CompareAndSwapPointer(
							(*unsafe.Pointer)(unsafe.Pointer(&w.nextGrid[nIdx])),
							nil,
							unsafe.Pointer(&nextC)) {

							moved = true
							ate = true

							// 繁殖逻辑
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

				// 优先级 2: 没吃到鱼，像鱼一样移动
				if !ate {
					// 重新打乱方向 (或者继续使用上面的 perm，为了随机性更好建议重新打乱或继续遍历)
					// 简单起见，继续尝试移动到空位
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

			// 如果没有移动（被堵住了，或者没抢到位置），留在原地
			if !moved {
				// 尝试把 update 后的自己写入 nextGrid 的当前位置
				// 注意：如果是鲨鱼，能量已经减少了
				// 繁殖逻辑：不移动通常不繁殖（根据 Wa-Tor 规则，繁殖需要移动产生空位）
				atomic.CompareAndSwapPointer(
					(*unsafe.Pointer)(unsafe.Pointer(&w.nextGrid[idx])),
					nil,
					unsafe.Pointer(&nextC))
			}
		}
	}
}

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

		for i := 0; i < 50; i++ {
			world.Update()
		}
		runtime.GC()

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

func generateSpeedupReport(results map[int]time.Duration) {
	baseTime := results[1].Seconds()
	fmt.Println("\n=== Speedup Report ===")
	fmt.Printf("Baseline (1 Thread): %.4fs\n", baseTime)
	fmt.Println("Speedup relative to baseline:")

	for _, c := range []int{2, 4, 8} {
		if t, exists := results[c]; exists {
			ts := t.Seconds()
			speedup := baseTime / ts
			efficiency := (speedup / float64(c)) * 100
			fmt.Printf("%d Threads: %.4fs (Speedup: %.2fx, Efficiency: %.1f%%)\n",
				c, ts, speedup, efficiency)
		}
	}
}

type Game struct {
	world    *World
	fishImg  *ebiten.Image
	sharkImg *ebiten.Image
}

func NewGame() *Game {
	w := NewWorld(*windowWidth/pixSize, *windowHeight/pixSize, *threads)
	w.InitPopulation(*numSharks, *numFish)

	fishImg := ebiten.NewImage(pixSize, pixSize)
	fishImg.Fill(color.RGBA{80, 212, 80, 255})
	sharkImg := ebiten.NewImage(pixSize, pixSize)
	sharkImg.Fill(color.RGBA{100, 100, 252, 255})

	return &Game{world: w, fishImg: fishImg, sharkImg: sharkImg}
}

func (g *Game) Update() error {
	g.world.Update()
	return nil
}

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

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return g.world.screenWidth, g.world.screenHeight
}

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
