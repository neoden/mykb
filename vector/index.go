package vector

import (
	"math"
	"sort"
	"sync"
)

// Result represents a search result with chunk ID and similarity score.
type Result struct {
	ID    string  `json:"id"`
	Score float32 `json:"score"`
}

// Index is an in-memory vector index with brute-force search.
type Index struct {
	mu   sync.RWMutex
	vecs map[string][]float32
}

// NewIndex creates a new empty vector index.
func NewIndex() *Index {
	return &Index{
		vecs: make(map[string][]float32),
	}
}

// Load loads vectors from a map into the index.
func (idx *Index) Load(vecs map[string][]float32) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.vecs = vecs
}

// Add adds or updates a vector in the index.
func (idx *Index) Add(id string, vec []float32) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.vecs[id] = vec
}

// Remove removes a vector from the index.
func (idx *Index) Remove(id string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	delete(idx.vecs, id)
}

// Size returns the number of vectors in the index.
func (idx *Index) Size() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.vecs)
}

// Search finds the k most similar vectors to the query.
func (idx *Index) Search(query []float32, k int) []Result {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if len(idx.vecs) == 0 || k <= 0 {
		return nil
	}

	results := make([]Result, 0, len(idx.vecs))
	for id, vec := range idx.vecs {
		score := cosineSimilarity(query, vec)
		results = append(results, Result{ID: id, Score: score})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if k > len(results) {
		k = len(results)
	}
	return results[:k]
}

// cosineSimilarity computes the cosine similarity between two vectors.
func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}

	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return float32(dot / (math.Sqrt(normA) * math.Sqrt(normB)))
}
