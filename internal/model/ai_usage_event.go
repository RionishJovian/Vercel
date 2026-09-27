package model

import "time"

// AIUsageEvent is one AI call recorded by the platform.
//
// `feature` and `model` are FREE TEXT here — there is no CHECK constraint on
// either column, and the distinct values come from live data (six features, eight
// models in the migrated set). So an unknown filter value is NOT an error: it
// honestly means "no such feature", and returns an empty page. That is the
// opposite of the syslog/scan case, where the values ARE an enumerated CHECK and
// a bad filter must be a 400 rather than a silent empty result.
type AIUsageEvent struct {
	ID               string    `json:"id"`
	BusinessID       string    `json:"business_id"`
	UserID           *string   `json:"user_id,omitempty"`
	Feature          string    `json:"feature"`
	Model            string    `json:"model"`
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	TotalTokens      int       `json:"total_tokens"`
	Cached           bool      `json:"cached"`
	CreatedAt        time.Time `json:"created_at"`
	// TokenMismatch is true when TotalTokens is not PromptTokens+CompletionTokens.
	// Exactly one migrated row has this (831+6 recorded as 1027 — 190 tokens the
	// old app never broke out). Exposed so the console shows the recorded figure
	// and notes the discrepancy instead of rendering contradictory arithmetic.
	TokenMismatch bool `json:"token_mismatch"`
}

// AIUsageSummary is the aggregate view over a business's AI usage.
type AIUsageSummary struct {
	Events           int            `json:"events"`
	PromptTokens     int            `json:"prompt_tokens"`
	CompletionTokens int            `json:"completion_tokens"`
	TotalTokens      int            `json:"total_tokens"`
	CachedEvents     int            `json:"cached_events"`
	MismatchedEvents int            `json:"mismatched_events"`
	ByFeature        []AIUsageGroup `json:"by_feature"`
	ByModel          []AIUsageGroup `json:"by_model"`
}

// AIUsageGroup is one row of a feature/model breakdown.
type AIUsageGroup struct {
	Name             string `json:"name"`
	Events           int    `json:"events"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens"`
}
