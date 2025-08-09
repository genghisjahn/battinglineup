package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
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

func main() {

	players, err := loadPlayersFromFile("player_files/sample.json")
	if err != nil {
		log.Fatalf("Failed to load players: %v", err)
	}
	gamecount := 0
	for {
		gamecount++
		game := baseball.Game{}
		rand.Seed(time.Now().UnixNano())

		batterIndex := 0
		for inning := 1; inning <= 9; inning++ {
			fmt.Println("Inning:", inning)
			runsStart := game.Runs
			outs := 0
			for outs < 3 {
				game.Field.AtBat = &players[batterIndex]
				result := players[batterIndex].PlateAppearance("right")
				fmt.Println(players[batterIndex].LastName + ", " + players[batterIndex].FirstName + ": " + result)
				switch result {
				case baseball.HIT_OUT:
					outs++
					if game.Field.FirstBase != nil && outs < 2 {
						if rand.Float64() < 0.11 { // ~11% DP rate when R1, <2 outs
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
			fmt.Println("Inning:", inning, "LOB:", lob, "runs:", game.Runs-runsStart, "total:", game.Runs)
			fmt.Println("----------")
		}
		finalLOB := game.LOB
		fmt.Printf("Final totals — Hits: %d, Runs: %d, LOB: %d\n", game.Hits, game.Runs, finalLOB)
		if game.Hits == 0 {
			fmt.Println("NO HITTER! GAME:" + fmt.Sprintf("%v", gamecount))
			break
		}
	}
}
