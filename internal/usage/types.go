package usage

import (
	"context"
	"time"
)

type TokenStats struct {
	InputTokens     int64 `json:"input_tokens"`
	OutputTokens    int64 `json:"output_tokens"`
	ReasoningTokens int64 `json:"reasoning_tokens"`
	CachedTokens    int64 `json:"cached_tokens"`
	TotalTokens     int64 `json:"total_tokens"`
}

type Record struct {
	ID                 string
	Timestamp          time.Time
	APIKey             string
	Provider           string
	Model              string
	Source             string
	AuthIndex          string
	AuthType           string
	Endpoint           string
	RequestID          string
	LatencyMs          int64
	FirstByteLatencyMs int64
	GenerationMs       int64
	ThinkingEffort     string
	ServiceTier        string
	Tokens             TokenStats
	Failed             bool
	FailStatusCode     int
	FailBody           string
}

type RequestDetail struct {
	ID                 string     `json:"id"`
	Timestamp          time.Time  `json:"timestamp"`
	LatencyMs          int64      `json:"latency_ms"`
	FirstByteLatencyMs int64      `json:"first_byte_latency_ms"`
	GenerationMs       int64      `json:"generation_ms"`
	Source             string     `json:"source"`
	AuthIndex          string     `json:"auth_index"`
	ThinkingEffort     string     `json:"thinking_effort"`
	ServiceTier        string     `json:"service_tier"`
	Tokens             TokenStats `json:"tokens"`
	Failed             bool       `json:"failed"`
	FailStatusCode     int        `json:"fail_status_code,omitempty"`
	FailBody           string     `json:"fail_body,omitempty"`
}

type APIUsage map[string]map[string][]RequestDetail

type QueryRange struct {
	Start *time.Time
	End   *time.Time
}

type DeleteResult struct {
	Deleted int64    `json:"deleted"`
	Missing []string `json:"missing"`
}

type Store interface {
	Insert(ctx context.Context, record Record) error
	Query(ctx context.Context, rng QueryRange) (APIUsage, error)
	Delete(ctx context.Context, ids []string) (DeleteResult, error)
	Close() error
}
