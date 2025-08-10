package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"sync"
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

	// Loop over all possible 9-player lineups
	count := 0
	combinations(len(players), 9, func(idx []int) bool {
		permutations(idx, func(order []int) bool {
			lineup := make([]baseball.Player, 9)
			for i := 0; i < 9; i++ {
				lineup[i] = players[order[i]]
			}

			// This is where you'll put your simulation logic later

			count++
			if count%100000 == 0 {
				fmt.Printf("Processed %d permutations...\n", count)
			}
			return true
		})
		return true
	})
	return
	rand.Seed(time.Now().UnixNano())
	var wg sync.WaitGroup
	for gamecount := 0; gamecount < 1000; gamecount++ {
		wg.Add(1)
		go func(gameID int) {
			defer wg.Done()
			game := baseball.Game{}
			seed := uint64(time.Now().UnixNano())
			seed ^= 6364136223846793005 * uint64(gameID+1) // Knuth multiplicative mix, fits in uint64
			r := rand.New(rand.NewSource(int64(seed)))

			batterIndex := 0
			for inning := 1; inning <= 9; inning++ {
				//fmt.Println("Inning:", inning)
				outs := 0
				for outs < 3 {
					game.Field.AtBat = &players[batterIndex]
					result := players[batterIndex].PlateAppearance("right", r)
					//fmt.Println(players[batterIndex].LastName + ", " + players[batterIndex].FirstName + ": " + result)
					switch result {
					case baseball.HIT_OUT:
						outs++
						if game.Field.FirstBase != nil && outs < 2 {
							if r.Float64() < 0.11 { // ~11% DP rate when R1, <2 outs
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
				//fmt.Println("Inning:", inning, "LOB:", lob, "runs:", game.Runs-runsStart, "total:", game.Runs)
				//fmt.Println("----------")
			}
			finalLOB := game.LOB
			_ = finalLOB
			//fmt.Printf("Final totals — Hits: %d, Runs: %d, LOB: %d\n", game.Hits, game.Runs, finalLOB)
			if game.Hits == 0 {
				fmt.Println("NO HITTER! GAME:" + fmt.Sprintf("%v", gameID))
			}
		}(gamecount)
	}
	wg.Wait()
}
