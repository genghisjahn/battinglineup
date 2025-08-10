package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	baseball "github.com/genghisjahn/battinglineup/batting"
)

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

func main() {

	players, err := loadPlayersFromFile("player_files/phillies.json")
	if err != nil {
		log.Fatalf("Failed to load players: %v", err)
	}

	if len(players) < 9 {
		log.Fatalf("Need at least 9 players, have %d", len(players))
	}

	lineupCount := 1

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
				for g := 0; g < lineupCount; g++ {
					// --- Begin single-game simulation for this lineup ---
					game := baseball.Game{}
					batterIndex := 0
					for inning := 1; inning <= 9; inning++ {
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
				}
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
}
