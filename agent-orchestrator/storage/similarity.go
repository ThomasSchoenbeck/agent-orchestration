// Package storage provides semantic context storage and retrieval.
package storage

import "math"

// CosineSimilarity computes the cosine similarity between two float32 vectors.
// Returns 0 if either vector is zero-length or empty.
func CosineSimilarity(a, b []float32) float32 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		ai := float64(a[i])
		bi := float64(b[i])
		dot += ai * bi
		normA += ai * ai
		normB += bi * bi
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return float32(dot / (math.Sqrt(normA) * math.Sqrt(normB)))
}

// RankedEntry pairs a context entry ID and content with its similarity score.
type RankedEntry struct {
	ID         string
	ProjectID  string
	TaskID     string
	Type       string
	Content    string
	Embedding  []float32
	Similarity float32
}

// RankBySimilarity sorts entries by descending cosine similarity to the query embedding
// and returns the top-K results.
func RankBySimilarity(query []float32, entries []RankedEntry, topK int) []RankedEntry {
	// Score all entries.
	for i := range entries {
		entries[i].Similarity = CosineSimilarity(query, entries[i].Embedding)
	}
	// Partial sort: bubble the top-K to the front.
	n := len(entries)
	if topK <= 0 || topK > n {
		topK = n
	}
	for i := 0; i < topK; i++ {
		maxIdx := i
		for j := i + 1; j < n; j++ {
			if entries[j].Similarity > entries[maxIdx].Similarity {
				maxIdx = j
			}
		}
		entries[i], entries[maxIdx] = entries[maxIdx], entries[i]
	}
	return entries[:topK]
}
