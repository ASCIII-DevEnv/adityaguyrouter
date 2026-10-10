package storage

import (
	"sync"
	"time"
)

type InspectorRecord struct {
	ID           string        `json:"id"`
	Timestamp    time.Time     `json:"timestamp"`
	Endpoint     string        `json:"endpoint"`
	Method       string        `json:"method"`
	Model        string        `json:"model"`
	Account      string        `json:"account"`
	StatusCode   int           `json:"statusCode"`
	DurationMs   int64         `json:"durationMs"`
	RequestBody  string        `json:"requestBody"`
	ResponseBody string        `json:"responseBody"`
	PromptTokens int           `json:"promptTokens"`
	OutputTokens int           `json:"outputTokens"`
	IsStreaming  bool          `json:"isStreaming"`
	Error        string        `json:"error,omitempty"`
}

type RequestInspector struct {
	mu      sync.RWMutex
	records []InspectorRecord
	maxCap  int
}

func NewRequestInspector(capacity int) *RequestInspector {
	if capacity <= 0 {
		capacity = 50
	}
	return &RequestInspector{
		records: make([]InspectorRecord, 0, capacity),
		maxCap:  capacity,
	}
}

func (ri *RequestInspector) Record(rec InspectorRecord) {
	ri.mu.Lock()
	defer ri.mu.Unlock()

	if rec.Timestamp.IsZero() {
		rec.Timestamp = time.Now()
	}

	// Truncate gigantic request and response payloads to prevent megabyte browser transfer latency
	const maxPayloadPreview = 16384 // 16 KB max for preview & curl replay
	if len(rec.RequestBody) > maxPayloadPreview {
		rec.RequestBody = rec.RequestBody[:maxPayloadPreview]
	}
	if len(rec.ResponseBody) > maxPayloadPreview {
		rec.ResponseBody = rec.ResponseBody[:maxPayloadPreview]
	}

	// Insert at beginning (newest first)
	ri.records = append([]InspectorRecord{rec}, ri.records...)
	if len(ri.records) > ri.maxCap {
		ri.records = ri.records[:ri.maxCap]
	}
}

func (ri *RequestInspector) GetRecords() []InspectorRecord {
	ri.mu.RLock()
	defer ri.mu.RUnlock()

	copied := make([]InspectorRecord, len(ri.records))
	copy(copied, ri.records)
	return copied
}

func (ri *RequestInspector) Clear() {
	ri.mu.Lock()
	defer ri.mu.Unlock()
	ri.records = make([]InspectorRecord, 0, ri.maxCap)
}
