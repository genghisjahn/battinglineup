package main

import (
	"container/heap"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io/ioutil"
	"log"
	"math/rand"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	baseball "github.com/genghisjahn/battinglineup/batting"
)

// Agg holds aggregate stats per unique lineup key.
type Agg struct {
	Games int64
	Runs  int64
	Hits  int64
}

// lineupStats maps lineup hash -> aggregates. Safe for concurrent use.
var lineupStats sync.Map

// lineupResult holds summary for a single ordered lineup.
type lineupResult struct {
	Mean  float64
	Order []string
	Hash  uint64
}

// min-heap by Median
type resultHeap []lineupResult

func (h resultHeap) Len() int            { return len(h) }
func (h resultHeap) Less(i, j int) bool  { return h[i].Mean < h[j].Mean }
func (h resultHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *resultHeap) Push(x interface{}) { *h = append(*h, x.(lineupResult)) }
func (h *resultHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

var (
	topK    = 256
	hmu     sync.Mutex
	topHeap resultHeap
)

const bottomK = 10

var bmu sync.Mutex

type maxResultHeap []lineupResult

func (h maxResultHeap) Len() int            { return len(h) }
func (h maxResultHeap) Less(i, j int) bool  { return h[i].Mean > h[j].Mean } // max-heap by Mean
func (h maxResultHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *maxResultHeap) Push(x interface{}) { *h = append(*h, x.(lineupResult)) }
func (h *maxResultHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

var bottomHeap maxResultHeap

func loadPlayersFromFile(filePath string) ([]baseball.Player, error) {
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	var players []baseball.Player
	if err := json.Unmarshal(data, &players); err != nil {
		return nil, err
	}
	return players, nil
}

// combinations generates all k-combinations of numbers 0..n-1.
// For each combination, it calls yield with a slice of indices.
// If yield returns false, iteration stops.
func combinations(n, k int, yield func([]int) bool) {
	idx := make([]int, k)
	var rec func(int, int) bool
	rec = func(i, start int) bool {
		if i == k {
			comb := make([]int, k)
			copy(comb, idx)
			return yield(comb)
		}
		for s := start; s <= n-(k-i); s++ {
			idx[i] = s
			if !rec(i+1, s+1) {
				return false
			}
		}
		return true
	}
	rec(0, 0)
}

// permutations generates all permutations of a slice of indices.
// For each permutation, it calls yield with the permuted indices.
// If yield returns false, iteration stops.
func permutations(idx []int, yield func([]int) bool) {
	perm := make([]int, len(idx))
	copy(perm, idx)
	var rec func(int) bool
	rec = func(i int) bool {
		if i == len(perm) {
			p := make([]int, len(perm))
			copy(p, perm)
			return yield(p)
		}
		for j := i; j < len(perm); j++ {
			perm[i], perm[j] = perm[j], perm[i]
			if !rec(i + 1) {
				perm[i], perm[j] = perm[j], perm[i]
				return false
			}
			perm[i], perm[j] = perm[j], perm[i]
		}
		return true
	}
	rec(0)
}

// lineupHash returns a stable 64-bit FNV-1a hash for the ordered 9-player lineup.
// It incorporates batting ORDER and uses LastName,FirstName for identity.
func lineupHash(lineup []baseball.Player) uint64 {
	h := fnv.New64a()
	// Build a compact key like: 0:Last,First|1:Last,First|...|8:Last,First
	var b strings.Builder
	b.Grow(9 * 20) // heuristic to reduce reallocs
	for i := 0; i < len(lineup); i++ {
		if i > 0 {
			b.WriteByte('|')
		}
		b.WriteString(fmt.Sprintf("%d:%s,%s", i, lineup[i].LastName, lineup[i].FirstName))
	}
	h.Write([]byte(b.String()))
	return h.Sum64()
}

func main() {

	players, err := loadPlayersFromFile("player_files/phillies.json")
	if err != nil {
		log.Fatalf("Failed to load players: %v", err)
	}

	if len(players) < 9 {
		log.Fatalf("Need at least 9 players, have %d", len(players))
	}

	lineupCount := 200

	// Concurrent lineup processing
	lineupCh := make(chan []baseball.Player, 1024)
	var wg sync.WaitGroup
	var count uint64
	workers := runtime.NumCPU()

	// Start workers
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func(workerID int) {
			defer wg.Done()
			r := rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID)*9973))
			for lineup := range lineupCh {
				// Compute unique key for this ordered lineup
				hash := lineupHash(lineup)
				orderNames := make([]string, 9)
				for i := 0; i < 9; i++ {
					orderNames[i] = lineup[i].LastName
				}
				runs := make([]int, 0, lineupCount)
				var runsSum, hitsSum int64
				for g := 0; g < lineupCount; g++ {
					// --- Begin single-game simulation for this lineup ---
					game := baseball.Game{}
					game.StartPitcher(r)
					var pitcherChanged bool
					batterIndex := 0
					for inning := 1; inning <= 9; inning++ {
						game.MaybeChangePitcher(inning, &pitcherChanged, r)
						outs := 0
						for outs < 3 {
							game.Field.AtBat = &lineup[batterIndex]
							result := lineup[batterIndex].PlateAppearance("right", r)
							switch result {
							case baseball.HIT_OUT:
								outs++
								if game.Field.FirstBase != nil && outs < 2 {
									if r.Float64() < 0.11 {
										outs++
										game.Field.FirstBase = nil
									}
								}
							case baseball.HIT_BY_PITCH_WALK:
								game.Hit(baseball.HIT_BY_PITCH_WALK)
							case baseball.HIT_SINGLE:
								game.Hit(baseball.HIT_SINGLE)
							case baseball.HIT_DOUBLE:
								game.Hit(baseball.HIT_DOUBLE)
							case baseball.HIT_TRIPLE:
								game.Hit(baseball.HIT_TRIPLE)
							case baseball.HIT_HOMERUN:
								game.Hit(baseball.HIT_HOMERUN)
							}
							game.Field.AtBat = nil
							batterIndex++
							if batterIndex >= 9 {
								batterIndex = 0
							}
						}
						lob := game.Field.LOB()
						game.AddLOB(lob)
						game.Field.FirstBase, game.Field.SecondBase, game.Field.ThirdBase = nil, nil, nil
					}
					// --- End single-game simulation ---
					runs = append(runs, game.Runs)
					runsSum += int64(game.Runs)
					hitsSum += int64(game.Hits)
				}

				mean := float64(runsSum) / float64(lineupCount)

				// Maintain top-K by mean
				hmu.Lock()
				if len(topHeap) < topK {
					heap.Push(&topHeap, lineupResult{Mean: mean, Order: orderNames, Hash: hash})
				} else if topHeap[0].Mean < mean {
					heap.Pop(&topHeap)
					heap.Push(&topHeap, lineupResult{Mean: mean, Order: orderNames, Hash: hash})
				}
				hmu.Unlock()

				// Maintain bottom-K by mean
				bmu.Lock()
				if len(bottomHeap) < bottomK {
					heap.Push(&bottomHeap, lineupResult{Mean: mean, Order: orderNames, Hash: hash})
				} else if bottomHeap[0].Mean > mean {
					heap.Pop(&bottomHeap)
					heap.Push(&bottomHeap, lineupResult{Mean: mean, Order: orderNames, Hash: hash})
				}
				bmu.Unlock()

				// Update global aggregates once per lineup
				val, _ := lineupStats.LoadOrStore(hash, &Agg{})
				agg := val.(*Agg)
				atomic.AddInt64(&agg.Games, int64(lineupCount))
				atomic.AddInt64(&agg.Runs, runsSum)
				atomic.AddInt64(&agg.Hits, hitsSum)

				// Progress counter
				if atomic.AddUint64(&count, 1)%100000 == 0 {
					fmt.Printf("Processed %d permutations...\n", atomic.LoadUint64(&count))
				}
			}
		}(w)
	}

	// Loop over all possible 9-player lineups (generator feeding workers)
	go func() {
		combinations(len(players), 9, func(idx []int) bool {
			permutations(idx, func(order []int) bool {
				lineup := make([]baseball.Player, 9)
				for i := 0; i < 9; i++ {
					lineup[i] = players[order[i]]
				}
				lineupCh <- lineup
				return true
			})
			return true
		})
		close(lineupCh)
	}()

	wg.Wait()

	// Output top-K by mean runs
	hmu.Lock()
	results := make([]lineupResult, len(topHeap))
	copy(results, topHeap)
	hmu.Unlock()
	sort.Slice(results, func(i, j int) bool { return results[i].Mean > results[j].Mean })
	fmt.Println("Top lineups by average runs:")
	for i, r := range results {
		id := fmt.Sprintf("%x", r.Hash)[:6]
		fmt.Printf("%2d) ID=%s mean=%.3f  order=%v\n", i+1, id, r.Mean, r.Order)
	}

	// Output bottom-K by mean runs
	bmu.Lock()
	bresults := make([]lineupResult, len(bottomHeap))
	copy(bresults, bottomHeap)
	bmu.Unlock()

	sort.Slice(bresults, func(i, j int) bool { return bresults[i].Mean < bresults[j].Mean })
	fmt.Println("Bottom lineups by average runs:")
	for i, r := range bresults {
		id := fmt.Sprintf("%x", r.Hash)[:6]
		fmt.Printf("%2d) ID=%s mean=%.3f  order=%v\n", i+1, id, r.Mean, r.Order)
	}
}
