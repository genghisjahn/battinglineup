package baseball

import (
	"math/rand"
	"time"
)

const HIT_SINGLE = "single"
const HIT_DOUBLE = "double"
const HIT_TRIPLE = "triple"
const HIT_HOMERUN = "home_run"
const HIT_BY_PITCH_WALK = "walk_hbp"
const HIT_OUT = "out"

func init() {
	rand.Seed(time.Now().UnixNano())
}

type Player struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	LHP       Stats  `json:"LHP"`
	RHP       Stats  `json:"RHP"`
}

func (p Player) PlateAppearance(LRPitcher string) string {
	// Choose splits based on pitcher handedness input ("left" uses LHP, otherwise RHP)
	var s Stats
	if LRPitcher == "left" {
		s = p.LHP
	} else {
		s = p.RHP
	}

	r := rand.Float64()
	// Outcome by OBP/AVG thresholds
	if r > s.OBP {
		return HIT_OUT
	}
	if r > s.AVG { // r <= OBP here
		return HIT_BY_PITCH_WALK
	}
	// It's a hit: decide which kind
	return hitType(s.AVG, s.SLUG)
}

type Stats struct {
	AVG  float64 `json:"avg"`
	OBP  float64 `json:"obp"`
	SLUG float64 `json:"slug"`
}

type Field struct {
	AtBat      *Player `json:"at_bat"`
	FirstBase  *Player `json:"first_base"`
	SecondBase *Player `json:"second_base"`
	ThirdBase  *Player `json:"third_base"`
}

func (f Field) LOB() int {
	b := 0
	if f.FirstBase != nil {
		b++
	}
	if f.SecondBase != nil {
		b++
	}
	if f.ThirdBase != nil {
		b++
	}
	return b
}

func (g *Game) AddLOB(lob int) {
	g.LOB += lob
}

func (g *Game) Hit(hittype string) {
	if hittype == HIT_BY_PITCH_WALK {
		// Force-only advances on walk/HBP
		// If 1B is occupied, it forces runners forward; 3B only scores when bases are loaded.
		if g.Field.FirstBase != nil {
			if g.Field.SecondBase != nil {
				if g.Field.ThirdBase != nil {
					// Bases loaded: force in a run
					g.Field.ThirdBase = nil
					g.Runs++
				}
				// Force 2B -> 3B
				g.Field.ThirdBase = g.Field.SecondBase
				g.Field.SecondBase = nil
			}
			// Force 1B -> 2B
			g.Field.SecondBase = g.Field.FirstBase
			g.Field.FirstBase = nil
		}
		g.Field.FirstBase = g.Field.AtBat
		g.Field.AtBat = nil
	}
	if hittype == HIT_SINGLE {
		//TODO, sometimes a run will score from Second
		g.Hits++
		if g.Field.ThirdBase != nil {
			g.Field.ThirdBase = nil
			g.Runs++
		}
		if g.Field.SecondBase != nil {
			g.Field.ThirdBase = g.Field.SecondBase
			g.Field.SecondBase = nil
		}
		if g.Field.FirstBase != nil {
			g.Field.SecondBase = g.Field.FirstBase
			g.Field.FirstBase = nil
		}
		g.Field.FirstBase = g.Field.AtBat
		g.Field.AtBat = nil
	}
	if hittype == HIT_DOUBLE {
		//TODO, sometimes a run will score from First
		g.Hits++
		if g.Field.ThirdBase != nil {
			g.Field.ThirdBase = nil
			g.Runs++
		}
		if g.Field.SecondBase != nil {
			g.Field.SecondBase = nil
			g.Runs++
		}
		if g.Field.FirstBase != nil {
			g.Field.ThirdBase = g.Field.FirstBase
			g.Field.FirstBase = nil
		}
		g.Field.SecondBase = g.Field.AtBat
		g.Field.AtBat = nil
	}
	if hittype == HIT_TRIPLE {
		g.Hits++
		if g.Field.ThirdBase != nil {
			g.Field.ThirdBase = nil
			g.Runs++
		}
		if g.Field.SecondBase != nil {
			g.Field.SecondBase = nil
			g.Runs++
		}
		if g.Field.FirstBase != nil {
			g.Field.FirstBase = nil
			g.Runs++
		}
		g.Field.ThirdBase = g.Field.AtBat
		g.Field.AtBat = nil
	}
	if hittype == HIT_HOMERUN {
		g.Hits++
		if g.Field.ThirdBase != nil {
			g.Field.ThirdBase = nil
			g.Runs++
		}
		if g.Field.SecondBase != nil {
			g.Field.SecondBase = nil
			g.Runs++
		}
		if g.Field.FirstBase != nil {
			g.Field.FirstBase = nil
			g.Runs++
		}
		g.Runs++
		g.Field.AtBat = nil
	}
}

type Game struct {
	Hits  int
	Runs  int
	LOB   int
	Field Field
}

func hitType(avg, slug float64) string {
	// Defensive defaults
	if avg <= 0 || slug <= 0 {
		return HIT_SINGLE
	}

	// Average bases per hit
	t := slug / avg
	// Clamp to a realistic range so extreme inputs don't explode rates
	if t < 1.10 {
		t = 1.10
	}
	if t > 2.10 {
		t = 2.10
	}

	// MLB-ish baselines (roughly 70–76% 1B, 16–22% 2B, ~1–2% 3B, 4–10% HR)
	p3 := 0.015 // keep triples rare; nudge up slightly for big t
	if t > 1.70 {
		p3 = 0.02
	}

	// Target doubles share scales gently with power, but stays bounded
	p2 := 0.19 + 0.20*(t-1.55)
	if p2 < 0.12 {
		p2 = 0.12
	}
	if p2 > 0.26 {
		p2 = 0.26
	}

	// Given 1B=1 TB, 2B=2 TB, 3B=3 TB, HR=4 TB, and probabilities that sum to 1,
	// the remaining TB needed to hit `t` dictates a provisional HR rate.
	rem := t - (1 + p2 + 2*p3)
	pHR := rem / 3.0
	// Bound HR into a realistic band
	if pHR < 0.03 {
		pHR = 0.03
	}
	if pHR > 0.12 {
		pHR = 0.12
	}

	// Singles are whatever remains
	pS := 1.0 - (p2 + p3 + pHR)
	// Enforce a floor on singles share to avoid runaway extra-base explosions
	if pS < 0.55 {
		// Reduce HR first, then 2B, to restore singles floor
		deficit := 0.55 - pS
		// Reduce HR
		maxHRReduce := pHR - 0.03
		if maxHRReduce < 0 {
			maxHRReduce = 0
		}
		if deficit <= maxHRReduce {
			pHR -= deficit
			deficit = 0
		} else {
			pHR -= maxHRReduce
			deficit -= maxHRReduce
		}
		// Reduce 2B if needed
		if deficit > 0 {
			max2BReduce := p2 - 0.12
			if max2BReduce < 0 {
				max2BReduce = 0
			}
			if deficit <= max2BReduce {
				p2 -= deficit
				deficit = 0
			} else {
				p2 -= max2BReduce
				deficit -= max2BReduce
			}
		}
		pS = 1.0 - (p2 + p3 + pHR)
		if pS < 0 {
			pS = 0
		}
	}

	// Draw
	r := rand.Float64()
	if r < pS {
		return HIT_SINGLE
	}
	r -= pS
	if r < p2 {
		return HIT_DOUBLE
	}
	r -= p2
	if r < p3 {
		return HIT_TRIPLE
	}
	return HIT_HOMERUN
}
