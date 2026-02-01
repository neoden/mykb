package vector

import (
	"math"
	"testing"
)

func TestNewIndex(t *testing.T) {
	idx := NewIndex()
	if idx.Size() != 0 {
		t.Errorf("Size() = %d, want 0", idx.Size())
	}
}

func TestAddAndSize(t *testing.T) {
	idx := NewIndex()
	idx.Add("a", []float32{1, 0, 0})
	idx.Add("b", []float32{0, 1, 0})

	if idx.Size() != 2 {
		t.Errorf("Size() = %d, want 2", idx.Size())
	}
}

func TestRemove(t *testing.T) {
	idx := NewIndex()
	idx.Add("a", []float32{1, 0, 0})
	idx.Add("b", []float32{0, 1, 0})
	idx.Remove("a")

	if idx.Size() != 1 {
		t.Errorf("Size() = %d, want 1", idx.Size())
	}
}

func TestLoad(t *testing.T) {
	idx := NewIndex()
	vecs := map[string][]float32{
		"a": {1, 0, 0},
		"b": {0, 1, 0},
		"c": {0, 0, 1},
	}
	idx.Load(vecs)

	if idx.Size() != 3 {
		t.Errorf("Size() = %d, want 3", idx.Size())
	}
}

func TestSearchEmpty(t *testing.T) {
	idx := NewIndex()
	results := idx.Search([]float32{1, 0, 0}, 10)

	if len(results) != 0 {
		t.Errorf("len(results) = %d, want 0", len(results))
	}
}

func TestSearchFindsExactMatch(t *testing.T) {
	idx := NewIndex()
	idx.Add("a", []float32{1, 0, 0})
	idx.Add("b", []float32{0, 1, 0})
	idx.Add("c", []float32{0, 0, 1})

	results := idx.Search([]float32{1, 0, 0}, 1)

	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].ID != "a" {
		t.Errorf("results[0].ID = %q, want %q", results[0].ID, "a")
	}
	if results[0].Score < 0.99 {
		t.Errorf("results[0].Score = %f, want ~1.0", results[0].Score)
	}
}

func TestSearchReturnsTopK(t *testing.T) {
	idx := NewIndex()
	idx.Add("a", []float32{1, 0, 0})
	idx.Add("b", []float32{0.9, 0.1, 0})
	idx.Add("c", []float32{0.8, 0.2, 0})
	idx.Add("d", []float32{0, 1, 0})

	results := idx.Search([]float32{1, 0, 0}, 2)

	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	// First should be exact match
	if results[0].ID != "a" {
		t.Errorf("results[0].ID = %q, want %q", results[0].ID, "a")
	}
	// Second should be closest
	if results[1].ID != "b" {
		t.Errorf("results[1].ID = %q, want %q", results[1].ID, "b")
	}
}

func TestSearchKLargerThanSize(t *testing.T) {
	idx := NewIndex()
	idx.Add("a", []float32{1, 0, 0})
	idx.Add("b", []float32{0, 1, 0})

	results := idx.Search([]float32{1, 0, 0}, 100)

	if len(results) != 2 {
		t.Errorf("len(results) = %d, want 2", len(results))
	}
}

func TestSearchZeroK(t *testing.T) {
	idx := NewIndex()
	idx.Add("a", []float32{1, 0, 0})

	results := idx.Search([]float32{1, 0, 0}, 0)

	if results != nil {
		t.Errorf("results = %v, want nil", results)
	}
}

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		a, b []float32
		want float32
	}{
		{[]float32{1, 0, 0}, []float32{1, 0, 0}, 1.0},
		{[]float32{1, 0, 0}, []float32{0, 1, 0}, 0.0},
		{[]float32{1, 0, 0}, []float32{-1, 0, 0}, -1.0},
		{[]float32{1, 1, 0}, []float32{1, 0, 0}, float32(1 / math.Sqrt(2))},
	}

	for _, tt := range tests {
		got := cosineSimilarity(tt.a, tt.b)
		if math.Abs(float64(got-tt.want)) > 0.001 {
			t.Errorf("cosineSimilarity(%v, %v) = %f, want %f", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestCosineSimilarityDifferentLengths(t *testing.T) {
	got := cosineSimilarity([]float32{1, 0}, []float32{1, 0, 0})
	if got != 0 {
		t.Errorf("cosineSimilarity with different lengths = %f, want 0", got)
	}
}

func TestCosineSimilarityZeroVector(t *testing.T) {
	got := cosineSimilarity([]float32{0, 0, 0}, []float32{1, 0, 0})
	if got != 0 {
		t.Errorf("cosineSimilarity with zero vector = %f, want 0", got)
	}
}

func TestSearchDimensionMismatch(t *testing.T) {
	idx := NewIndex()
	idx.Add("dim3", []float32{1, 0, 0})       // 3 dimensions
	idx.Add("dim4", []float32{1, 0, 0, 0})    // 4 dimensions - mismatch
	idx.Add("dim3b", []float32{0.9, 0.1, 0})  // 3 dimensions

	// Search with 3-dim query should skip the 4-dim vector
	results := idx.Search([]float32{1, 0, 0}, 10)

	if len(results) != 2 {
		t.Errorf("len(results) = %d, want 2 (should skip mismatched dimension)", len(results))
	}

	// Check that dim4 is not in results
	for _, r := range results {
		if r.ID == "dim4" {
			t.Error("dim4 should be skipped due to dimension mismatch")
		}
	}
}
