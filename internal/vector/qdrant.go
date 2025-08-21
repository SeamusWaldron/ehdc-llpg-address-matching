package vector

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ehdc-llpg/internal/debug"
)

// QdrantClient implements vector database operations using Qdrant
type QdrantClient struct {
	baseURL    string
	httpClient *http.Client
	apiKey     string
}

// QdrantConfig holds Qdrant connection configuration
type QdrantConfig struct {
	Host    string
	Port    int
	APIKey  string
	Timeout time.Duration
}

// NewQdrantClient creates a new Qdrant client
func NewQdrantClient(config QdrantConfig) *QdrantClient {
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}

	return &QdrantClient{
		baseURL: fmt.Sprintf("http://%s:%d", config.Host, config.Port),
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		apiKey: config.APIKey,
	}
}

// CreateCollection creates a new collection in Qdrant for address embeddings
func (q *QdrantClient) CreateCollection(localDebug bool, collectionName string, vectorSize int) error {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)

	collection := map[string]interface{}{
		"vectors": map[string]interface{}{
			"size":     vectorSize,
			"distance": "Cosine", // Cosine distance for address similarity
		},
		"optimizers_config": map[string]interface{}{
			"default_segment_number": 2,
		},
		"hnsw_config": map[string]interface{}{
			"m":                 16,
			"ef_construct":      128,
			"full_scan_threshold": 10000,
		},
	}

	url := fmt.Sprintf("%s/collections/%s", q.baseURL, collectionName)
	return q.makeRequest(localDebug, "PUT", url, collection, nil)
}

// UpsertPoints inserts or updates vectors in the collection
func (q *QdrantClient) UpsertPoints(localDebug bool, collectionName string, points []QdrantPoint) error {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)

	payload := map[string]interface{}{
		"points": points,
	}

	url := fmt.Sprintf("%s/collections/%s/points", q.baseURL, collectionName)
	debug.DebugOutput(localDebug, "Upserting %d points to collection %s", len(points), collectionName)
	
	return q.makeRequest(localDebug, "PUT", url, payload, nil)
}

// SearchPoints performs vector similarity search
func (q *QdrantClient) SearchPoints(localDebug bool, collectionName string, vector []float32, limit int, threshold float64) ([]QdrantSearchResult, error) {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)

	query := map[string]interface{}{
		"vector": vector,
		"limit":  limit,
		"with_payload": true,
		"with_vector":  false,
	}

	if threshold > 0 {
		query["score_threshold"] = threshold
	}

	url := fmt.Sprintf("%s/collections/%s/points/search", q.baseURL, collectionName)
	
	var response QdrantSearchResponse
	err := q.makeRequest(localDebug, "POST", url, query, &response)
	if err != nil {
		return nil, err
	}

	debug.DebugOutput(localDebug, "Search returned %d results", len(response.Result))
	return response.Result, nil
}

// GetPoint retrieves a specific point by ID
func (q *QdrantClient) GetPoint(localDebug bool, collectionName string, pointID interface{}) (*QdrantPoint, error) {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)

	url := fmt.Sprintf("%s/collections/%s/points/%v", q.baseURL, collectionName, pointID)
	
	var response QdrantGetResponse
	err := q.makeRequest(localDebug, "GET", url, nil, &response)
	if err != nil {
		return nil, err
	}

	return &response.Result, nil
}

// DeletePoints removes points from the collection
func (q *QdrantClient) DeletePoints(localDebug bool, collectionName string, pointIDs []interface{}) error {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)

	payload := map[string]interface{}{
		"points": pointIDs,
	}

	url := fmt.Sprintf("%s/collections/%s/points/delete", q.baseURL, collectionName)
	return q.makeRequest(localDebug, "POST", url, payload, nil)
}

// CollectionInfo gets information about a collection
func (q *QdrantClient) CollectionInfo(localDebug bool, collectionName string) (*QdrantCollectionInfo, error) {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)

	url := fmt.Sprintf("%s/collections/%s", q.baseURL, collectionName)
	
	var response QdrantCollectionResponse
	err := q.makeRequest(localDebug, "GET", url, nil, &response)
	if err != nil {
		return nil, err
	}

	return &response.Result, nil
}

// makeRequest handles HTTP requests to Qdrant API
func (q *QdrantClient) makeRequest(localDebug bool, method, url string, payload interface{}, result interface{}) error {
	var body io.Reader

	if payload != nil {
		jsonData, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		body = bytes.NewBuffer(jsonData)
		debug.DebugOutput(localDebug, "Request payload size: %d bytes", len(jsonData))
	}

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if q.apiKey != "" {
		req.Header.Set("api-key", q.apiKey)
	}

	debug.DebugOutput(localDebug, "Making %s request to %s", method, url)

	resp, err := q.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	debug.DebugOutput(localDebug, "Response status: %s, body size: %d bytes", resp.Status, len(respBody))

	if resp.StatusCode >= 400 {
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	if result != nil {
		err = json.Unmarshal(respBody, result)
		if err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}
	}

	return nil
}

// QdrantPoint represents a point in Qdrant vector space
type QdrantPoint struct {
	ID      interface{}            `json:"id"`
	Vector  []float32              `json:"vector"`
	Payload map[string]interface{} `json:"payload"`
}

// QdrantSearchResult represents a search result from Qdrant
type QdrantSearchResult struct {
	ID      interface{}            `json:"id"`
	Score   float64               `json:"score"`
	Payload map[string]interface{} `json:"payload"`
	Vector  []float32             `json:"vector,omitempty"`
}

// QdrantSearchResponse represents the response from a search query
type QdrantSearchResponse struct {
	Result []QdrantSearchResult `json:"result"`
	Status string               `json:"status"`
	Time   float64              `json:"time"`
}

// QdrantGetResponse represents the response from getting a single point
type QdrantGetResponse struct {
	Result QdrantPoint `json:"result"`
	Status string      `json:"status"`
	Time   float64     `json:"time"`
}

// QdrantCollectionInfo represents collection metadata
type QdrantCollectionInfo struct {
	Status        string                 `json:"status"`
	OptimizerStatus string               `json:"optimizer_status"`
	VectorsCount  int64                 `json:"vectors_count"`
	IndexedVectorsCount int64           `json:"indexed_vectors_count"`
	PointsCount   int64                 `json:"points_count"`
	SegmentsCount int                   `json:"segments_count"`
	Config        map[string]interface{} `json:"config"`
}

// QdrantCollectionResponse represents the response when getting collection info
type QdrantCollectionResponse struct {
	Result QdrantCollectionInfo `json:"result"`
	Status string              `json:"status"`
	Time   float64             `json:"time"`
}

// AddressVectorDB implements the VectorDB interface using Qdrant
type AddressVectorDB struct {
	client         *QdrantClient
	collectionName string
	vectorSize     int
}

// NewAddressVectorDB creates a new address vector database using Qdrant
func NewAddressVectorDB(client *QdrantClient, collectionName string, vectorSize int) *AddressVectorDB {
	return &AddressVectorDB{
		client:         client,
		collectionName: collectionName,
		vectorSize:     vectorSize,
	}
}

// Initialize sets up the vector database collection
func (avdb *AddressVectorDB) Initialize(localDebug bool) error {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)

	// Try to get collection info first
	_, err := avdb.client.CollectionInfo(localDebug, avdb.collectionName)
	if err != nil {
		// Collection doesn't exist, create it
		debug.DebugOutput(localDebug, "Creating collection %s with vector size %d", avdb.collectionName, avdb.vectorSize)
		return avdb.client.CreateCollection(localDebug, avdb.collectionName, avdb.vectorSize)
	}

	debug.DebugOutput(localDebug, "Collection %s already exists", avdb.collectionName)
	return nil
}

// IndexAddresses bulk loads address embeddings into the vector database
func (avdb *AddressVectorDB) IndexAddresses(localDebug bool, addresses []AddressEmbedding, batchSize int) error {
	debug.DebugHeader(localDebug)
	defer debug.DebugFooter(localDebug)

	debug.DebugOutput(localDebug, "Indexing %d addresses in batches of %d", len(addresses), batchSize)

	for i := 0; i < len(addresses); i += batchSize {
		end := i + batchSize
		if end > len(addresses) {
			end = len(addresses)
		}

		batch := addresses[i:end]
		points := make([]QdrantPoint, len(batch))

		for j, addr := range batch {
			points[j] = QdrantPoint{
				ID:     addr.UPRN,
				Vector: addr.Embedding,
				Payload: map[string]interface{}{
					"uprn":         addr.UPRN,
					"locaddress":   addr.LocAddress,
					"addr_can":     addr.AddrCanonical,
					"easting":      addr.Easting,
					"northing":     addr.Northing,
					"indexed_at":   time.Now().Unix(),
				},
			}
		}

		err := avdb.client.UpsertPoints(localDebug, avdb.collectionName, points)
		if err != nil {
			return fmt.Errorf("failed to upsert batch %d-%d: %w", i, end-1, err)
		}

		debug.DebugOutput(localDebug, "Indexed batch %d-%d", i, end-1)
	}

	return nil
}

// Query performs vector similarity search for addresses
func (avdb *AddressVectorDB) Query(vector []float32, limit int) ([]VectorResult, error) {
	results, err := avdb.client.SearchPoints(false, avdb.collectionName, vector, limit, 0.0)
	if err != nil {
		return nil, err
	}

	vectorResults := make([]VectorResult, len(results))
	for i, result := range results {
		uprn := ""
		if uprnVal, ok := result.Payload["uprn"]; ok {
			if uprnStr, ok := uprnVal.(string); ok {
				uprn = uprnStr
			}
		}
		
		vectorResults[i] = VectorResult{
			UPRN:  uprn,
			Score: result.Score,
		}
	}

	return vectorResults, nil
}

// GetVector retrieves the vector for a specific UPRN
func (avdb *AddressVectorDB) GetVector(uprn string) ([]float32, error) {
	point, err := avdb.client.GetPoint(false, avdb.collectionName, uprn)
	if err != nil {
		return nil, err
	}
	return point.Vector, nil
}

// AddressEmbedding represents an address with its vector embedding
type AddressEmbedding struct {
	UPRN          string
	LocAddress    string
	AddrCanonical string
	Easting       float64
	Northing      float64
	Embedding     []float32
}

// VectorResult represents the interface result type
type VectorResult struct {
	UPRN  string
	Score float64
}