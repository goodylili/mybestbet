package stats

import "math"

var ratingBucketEdges = []float64{0, 6, 6.5, 7, 7.5, 8, 10}

func ratingDistribution(ratings []float64) RatingDistribution {
	d := RatingDistribution{Ratings: ratings, Buckets: make([]RatingBucket, len(ratingBucketEdges)-1)}
	for i := range d.Buckets {
		d.Buckets[i] = RatingBucket{From: ratingBucketEdges[i], To: ratingBucketEdges[i+1]}
	}
	if len(ratings) == 0 {
		return d
	}
	d.Min, d.Max = math.Inf(1), math.Inf(-1)
	sum := 0.0
	for _, r := range ratings {
		sum += r
		d.Min, d.Max = math.Min(d.Min, r), math.Max(d.Max, r)
		d.Buckets[bucketIndex(r)].Count++
	}
	d.Mean = sum / float64(len(ratings))
	return d
}

func bucketIndex(r float64) int {
	for i := len(ratingBucketEdges) - 2; i > 0; i-- {
		if r >= ratingBucketEdges[i] {
			return i
		}
	}
	return 0
}
