package storage

import (
	"math"
	"testing"
)

func TestCosineSimilarity_Identical(t *testing.T) {
	a := []float32{1, 0, 0}
	got := CosineSimilarity(a, a)
	if math.Abs(float64(got-1.0)) > 1e-5 {
		t.Errorf("identical vectors: expected similarity 1.0, got %f", got)
	}
}

func TestCosineSimilarity_Orthogonal(t *testing.T) {
	a := []float32{1, 0, 0}
	b := []float32{0, 1, 0}
	got := CosineSimilarity(a, b)
	if math.Abs(float64(got)) > 1e-5 {
		t.Errorf("orthogonal vectors: expected similarity 0.0, got %f", got)
	}
}

func TestCosineSimilarity_Opposite(t *testing.T) {
	a := []float32{1, 0, 0}
	b := []float32{-1, 0, 0}
	got := CosineSimilarity(a, b)
	if math.Abs(float64(got+1.0)) > 1e-5 {
		t.Errorf("opposite vectors: expected similarity -1.0, got %f", got)
	}
}

func TestCosineSimilarity_EmptyVectors(t *testing.T) {
	got := CosineSimilarity([]float32{}, []float32{})
	if got != 0 {
		t.Errorf("empty vectors: expected 0, got %f", got)
	}
}

func TestCosineSimilarity_MismatchedLength(t *testing.T) {
	a := []float32{1, 2}
	b := []float32{1, 2, 3}
	got := CosineSimilarity(a, b)
	if got != 0 {
		t.Errorf("mismatched lengths: expected 0, got %f", got)
	}
}

func TestCosineSimilarity_ZeroVector(t *testing.T) {
	a := []float32{0, 0, 0}
	b := []float32{1, 2, 3}
	got := CosineSimilarity(a, b)
	if got != 0 {
		t.Errorf("zero vector: expected 0, got %f", got)
	}
}

func TestCosineSimilarity_Partial(t *testing.T) {
	// 45-degree angle → cos(45°) ≈ 0.7071
	a := []float32{1, 1, 0}
	b := []float32{1, 0, 0}
	got := CosineSimilarity(a, b)
	want := float32(1.0 / math.Sqrt(2))
	if math.Abs(float64(got-want)) > 1e-5 {
		t.Errorf("45° vectors: expected ~%f, got %f", want, got)
	}
}

func TestRankBySimilarity_TopK(t *testing.T) {
	query := []float32{1, 0, 0}
	entries := []RankedEntry{
		{ID: "a", Embedding: []float32{0, 1, 0}}, // sim ≈ 0
		{ID: "b", Embedding: []float32{1, 0, 0}}, // sim = 1
		{ID: "c", Embedding: []float32{1, 1, 0}}, // sim ≈ 0.707
		{ID: "d", Embedding: []float32{0, 0, 1}}, // sim = 0
	}
	result := RankBySimilarity(query, entries, 2)
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}
	if result[0].ID != "b" {
		t.Errorf("expected top result to be %q, got %q", "b", result[0].ID)
	}
	if result[1].ID != "c" {
		t.Errorf("expected second result to be %q, got %q", "c", result[1].ID)
	}
}

func TestRankBySimilarity_TopKExceedsLen(t *testing.T) {
	query := []float32{1, 0}
	entries := []RankedEntry{
		{ID: "x", Embedding: []float32{1, 0}},
		{ID: "y", Embedding: []float32{0, 1}},
	}
	result := RankBySimilarity(query, entries, 10)
	if len(result) != 2 {
		t.Errorf("expected all 2 entries when topK > len, got %d", len(result))
	}
}

func TestRankBySimilarity_ZeroTopK(t *testing.T) {
	query := []float32{1, 0}
	entries := []RankedEntry{
		{ID: "x", Embedding: []float32{1, 0}},
	}
	result := RankBySimilarity(query, entries, 0)
	// topK=0 treated as "all"
	if len(result) != 1 {
		t.Errorf("expected 1 result for topK=0, got %d", len(result))
	}
}

func TestRankBySimilarity_EmptyEntries(t *testing.T) {
	query := []float32{1, 0}
	result := RankBySimilarity(query, []RankedEntry{}, 5)
	if len(result) != 0 {
		t.Errorf("expected empty result for empty entries, got %d", len(result))
	}
}
