# Wa-Tor

A parallel implementation of the Wa-Tor ecological simulation in Go, featuring sharks and fish in a toroidal ocean world. This project demonstrates parallel computing concepts with performance analysis across multiple cores.

## Overview

This project implements the **Wa-Tor simulation** as described in A.K. Dewdney's Scientific American article "Sharks and Fish wage an ecological war on the toroidal planet of Wa-Tor". The simulation models predator-prey dynamics in a toroidal (wraparound) ocean world where sharks hunt fish according to specific ecological rules.

**Key Implementation Features:**
- Parallel processing using Go routines for optimized performance
- Toroidal grid world with wraparound boundaries
- Real-time graphical visualization using Ebiten
- Comprehensive performance benchmarking
- Thread-safe concurrent updates using atomic operations

## Features

- **Parallel Processing**: Utilizes multiple goroutines for efficient simulation updates
- **Real-time Visualization**: Interactive graphical display with customizable window size
- **Benchmark Mode**: Performance testing without graphics for accurate timing
- **Configurable Parameters**: Full control over simulation parameters via command-line flags
- **Doxygen Documentation**: Professionally documented code with API references
- **Performance Analysis**: Built-in speedup and efficiency calculations

## Installation

### Prerequisites

- **Go 1.19** or later
- **Linux** environment (as required by the project specification)
- **Git** for version control

### Steps

1. **Clone the repository**

```bash
git clone https://github.com/yourusername/wa-tor-simulation.git
cd wa-tor-simulation
```

2. **Install dependencies**

```bash
go mod download
```

3. **Build the project**

```bash
go build -o wa-tor
```

## Usage

### Graphical Simulation Mode

Run the simulation with real-time visualization:

```bash
go run .
```

Run the Benchmarks:

```bash
go run . -benchmark
```

## Parameters

The simulation accepts the following command-line parameters:

| Parameter | Default | Description |
|-----------|---------|-------------|
| `-NumShark` | 6000 | Starting population of sharks |
| `-NumFish` | 70000 | Starting population of fish |
| `-FishBreed` | 6 | Time units before fish can reproduce |
| `-SharkBreed` | 16 | Time units before sharks can reproduce |
| `-Starve` | 4 | Time units sharks can survive without food |
| `-GridSize` | 300 | Dimensions of the square world grid |
| `-Threads` | CPU cores | Number of concurrent goroutines to use |
| `-benchmark` | false | Run in benchmark mode (no graphics) |
| `-steps` | 4000 | Number of simulation steps for benchmark |
| `-width` | 1400 | Window width for graphical mode |
| `-height` | 900 | Window height for graphical mode |

## Performance Benchmarking

The project includes built-in performance analysis to measure speedup across different core counts.

### Expected Output Format

=== Wa-Tor Simulation Benchmark ===

Grid Size: 300, Sharks: 6000, Fish: 70000, Steps: 4000 <br>
Threads: 1, Time: 15.1875s <br>
Threads: 2, Time: 9.2899s <br>
Threads: 4, Time: 6.8986s <br>
Threads: 8, Time: 6.7240s <br>

=== Speedup Report ===

Speedup relative to baseline: <br>
1 Threads: 15.1875s (Speedup: 1.00x, Efficiency: 100.0%)
2 Threads: 9.2899s (Speedup: 1.63x, Efficiency: 81.7%) <br>
4 Threads: 6.8986s (Speedup: 2.20x, Efficiency: 55.0%) <br>
8 Threads: 6.7240s (Speedup: 2.26x, Efficiency: 28.2%) <br>

### Performance Characteristics

The simulation demonstrates effective scaling up to 2 threads, achieving a 1.63x speedup​ with high 81.7% efficiency. However, performance gains diminish significantly with more threads, reaching only a 2.26x speedup​ on 8 threads​ with low 28.2% efficiency. This indicates that beyond 2 threads, the computation becomes constrained by synchronization overhead​ and memory contention, making it increasingly memory-bound​.

## 📖 References

1. Dewdney, A.K. (1984). "Computer Recreations: Sharks and Fish wage an ecological war on the toroidal planet of Wa-Tor". *Scientific American*, pp. 14-22.
2. [Wa-Tor Wikipedia Page](https://en.wikipedia.org/wiki/Wa-Tor)
3. [Go Programming Language](https://golang.org/)
4. [Ebiten Game Library](https://ebiten.org/)

## Support

For questions or issues related to this implementation, please open an issue on the GitHub repository with detailed information about your environment and the problem encountered.

---

**Academic Project** - Developed as part of parallel computing curriculum requirements. Implementation satisfies all specified deliverables including parallel performance analysis and professional documentation.
