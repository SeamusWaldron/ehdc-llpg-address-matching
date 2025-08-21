package engine

import (
	"fmt"
	"math"
	"sort"
	"sync"
)

// InMemoryVectorDB provides a simple in-memory vector database
type InMemoryVectorDB struct {
	vectors map[string]*VectorEntry
	mutex   sync.RWMutex
}

// VectorEntry represents a stored vector with metadata
type VectorEntry struct {
	ID       string
	Vector   []float32
	Metadata map[string]interface{}
}

// NewInMemoryVectorDB creates a new in-memory vector database
func NewInMemoryVectorDB() *InMemoryVectorDB {
	return &InMemoryVectorDB{
		vectors: make(map[string]*VectorEntry),
	}
}

// Initialize initializes the vector database
func (vdb *InMemoryVectorDB) Initialize() error {
	vdb.mutex.Lock()
	defer vdb.mutex.Unlock()
	
	// Already initialized if we have vectors
	if len(vdb.vectors) > 0 {
		fmt.Printf("Vector database already initialized with %d vectors\n", len(vdb.vectors))
		return nil
	}
	
	fmt.Println("Initializing in-memory vector database...")
	vdb.vectors = make(map[string]*VectorEntry)
	return nil
}

// Store stores a vector with associated metadata
func (vdb *InMemoryVectorDB) Store(id string, vector []float32, metadata map[string]interface{}) error {
	if len(vector) == 0 {
		return fmt.Errorf("empty vector for id %s", id)
	}

	vdb.mutex.Lock()
	defer vdb.mutex.Unlock()

	// Normalize the vector
	normalized := normalizeVector(vector)

	vdb.vectors[id] = &VectorEntry{
		ID:       id,
		Vector:   normalized,
		Metadata: metadata,
	}

	return nil
}

// Search finds the most similar vectors to the query vector
func (vdb *InMemoryVectorDB) Search(queryVector []float32, limit int, minScore float64) ([]*VectorMatch, error) {
	if len(queryVector) == 0 {
		return nil, fmt.Errorf("empty query vector")
	}

	vdb.mutex.RLock()
	defer vdb.mutex.RUnlock()

	if len(vdb.vectors) == 0 {
		return nil, fmt.Errorf("no vectors stored in database")
	}

	// Normalize query vector
	normalizedQuery := normalizeVector(queryVector)

	// Calculate similarities
	type scoreEntry struct {
		match *VectorMatch
		score float64
	}

	var scores []scoreEntry
	for _, entry := range vdb.vectors {
		similarity := cosineSimilarity(normalizedQuery, entry.Vector)
		
		if similarity >= minScore {
			match := &VectorMatch{
				ID:       entry.ID,
				Score:    similarity,
				Metadata: entry.Metadata,
			}
			scores = append(scores, scoreEntry{match: match, score: similarity})
		}
	}

	// Sort by similarity (descending)
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].score > scores[j].score
	})

	// Return top matches
	var results []*VectorMatch
	for i, entry := range scores {
		if i >= limit {
			break
		}
		results = append(results, entry.match)
	}

	return results, nil
}

// GetStats returns statistics about the vector database
func (vdb *InMemoryVectorDB) GetStats() map[string]interface{} {
	vdb.mutex.RLock()
	defer vdb.mutex.RUnlock()

	stats := map[string]interface{}{
		"total_vectors": len(vdb.vectors),
		"database_type": "in_memory",
	}

	if len(vdb.vectors) > 0 {
		// Get dimension from first vector
		for _, entry := range vdb.vectors {
			stats["vector_dimension"] = len(entry.Vector)
			break
		}
	}

	return stats
}

// Clear removes all vectors from the database
func (vdb *InMemoryVectorDB) Clear() error {
	vdb.mutex.Lock()
	defer vdb.mutex.Unlock()

	vdb.vectors = make(map[string]*VectorEntry)
	return nil
}

// normalizeVector normalizes a vector to unit length
func normalizeVector(vector []float32) []float32 {
	var magnitude float64
	for _, val := range vector {
		magnitude += float64(val * val)
	}
	
	magnitude = math.Sqrt(magnitude)
	if magnitude == 0 {
		return vector // Avoid division by zero
	}

	normalized := make([]float32, len(vector))
	for i, val := range vector {
		normalized[i] = float32(float64(val) / magnitude)
	}

	return normalized
}

// cosineSimilarity calculates cosine similarity between two normalized vectors
func cosineSimilarity(vec1, vec2 []float32) float64 {
	if len(vec1) != len(vec2) {
		return 0.0
	}

	var dotProduct float64
	for i := range vec1 {
		dotProduct += float64(vec1[i] * vec2[i])
	}

	// Since vectors are normalized, cosine similarity is just the dot product
	// Clamp to [0, 1] for similarity (from [-1, 1] cosine range)
	similarity := (dotProduct + 1.0) / 2.0
	
	if similarity < 0 {
		similarity = 0
	}
	if similarity > 1 {
		similarity = 1
	}

	return similarity
}

// PersistentVectorDB interface for future database-backed implementations
type PersistentVectorDB interface {
	VectorDatabase
	Save(filepath string) error
	Load(filepath string) error
}