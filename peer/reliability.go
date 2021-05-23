package peer

func ComputeReliability(interactions []float64) float64 {
	// TODO: improve reliability computation
	return average(interactions)
}

func average(xs []float64) float64 {
	total := 0.0
	for _, v := range xs {
		total += v
	}
	return total / float64(len(xs))
}
