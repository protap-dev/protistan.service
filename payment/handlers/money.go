package handlers

func amountCentsToMajorUnits(amountCents int64) float64 {
	return float64(amountCents) / 100.0
}
