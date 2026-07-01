package routing

import (
	"math"

	"finding.vendor.com/db"
)

func ChooseVendor(eligibles []db.EligibleVendor, loads map[int]int) db.EligibleVendor {
	best := eligibles[0]
	bestLoad := loads[best.ID]
	for _, v := range eligibles[1:] {
		if loads[v.ID] < bestLoad {
			best = v
			bestLoad = loads[v.ID]
		}
	}
	return best
}

func ApplyDiscount(cartTotal, threshold, rate float64) (applied bool, amount, final float64) {
	if cartTotal > threshold {
		amount = round2(cartTotal * rate)
		return true, amount, round2(cartTotal - amount)
	}
	return false, 0, round2(cartTotal)
}

func round2(x float64) float64 { return math.Round(x*100) / 100 }
