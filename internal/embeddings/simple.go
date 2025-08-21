package embeddings

import (
	"crypto/md5"
	"fmt"
	"math"
	"strings"
)

// SimpleEmbedder creates basic embeddings from text
type SimpleEmbedder struct {
	dimensions int
}

// NewSimpleEmbedder creates a simple embedder
func NewSimpleEmbedder(dimensions int) *SimpleEmbedder {
	return &SimpleEmbedder{dimensions: dimensions}
}

// Embed creates a simple vector representation of text
func (se *SimpleEmbedder) Embed(text string) ([]float32, error) {
	if text == "" {
		return make([]float32, se.dimensions), nil
	}
	
	// Normalize text
	text = strings.ToUpper(strings.TrimSpace(text))
	
	// Create hash-based embedding
	hash := md5.Sum([]byte(text))
	
	// Convert hash to vector
	vector := make([]float32, se.dimensions)
	
	// Use hash bytes to seed vector values
	for i := 0; i < se.dimensions; i++ {
		hashIndex := i % len(hash)
		// Convert byte to float in range [-1, 1]
		vector[i] = (float32(hash[hashIndex])/255.0)*2.0 - 1.0
	}
	
	// Add token-based features for better semantic representation
	tokens := strings.Fields(text)
	if len(tokens) > 0 {
		// Encode token count
		tokenCount := float32(len(tokens))
		if se.dimensions > 10 {
			vector[se.dimensions-1] = tokenCount / 20.0 // Normalize
		}
		
		// Encode text length
		textLength := float32(len(text))
		if se.dimensions > 11 {
			vector[se.dimensions-2] = textLength / 100.0 // Normalize
		}
		
		// Encode presence of common address terms
		addressTerms := []string{"ROAD", "STREET", "AVENUE", "LANE", "CLOSE", "DRIVE", "GARDENS", "COURT"}
		termCount := 0
		for _, term := range addressTerms {
			if strings.Contains(text, term) {
				termCount++
			}
		}
		if se.dimensions > 12 {
			vector[se.dimensions-3] = float32(termCount) / float32(len(addressTerms))
		}
		
		// Encode numeric content (house numbers)
		numericCount := 0
		for _, token := range tokens {
			for _, char := range token {
				if char >= '0' && char <= '9' {
					numericCount++
					break
				}
			}
		}
		if se.dimensions > 13 {
			vector[se.dimensions-4] = float32(numericCount) / float32(len(tokens))
		}
	}
	
	// Normalize vector to unit length
	var norm float32
	for _, val := range vector {
		norm += val * val
	}
	norm = float32(math.Sqrt(float64(norm)))
	
	if norm > 0 {
		for i := range vector {
			vector[i] /= norm
		}
	}
	
	return vector, nil
}

// NoOpVectorDB implements VectorDB interface for testing without real vector database
type NoOpVectorDB struct{}

func NewNoOpVectorDB() *NoOpVectorDB {
	return &NoOpVectorDB{}
}

func (nv *NoOpVectorDB) Query(vector []float32, limit int) ([]VectorResult, error) {
	// Return empty results for now - can be enhanced later
	return []VectorResult{}, nil
}

// VectorResult for interface compatibility
type VectorResult struct {
	UPRN  string
	Score float64
}

func (nv *NoOpVectorDB) GetVector(uprn string) ([]float32, error) {
	// Return empty vector for now
	return []float32{}, fmt.Errorf("vector not found")
}

