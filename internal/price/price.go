// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Javier Santacruz

// Package price turns a model reply's tokens into dollars.
package price

// Tokens of one reply, or a sum of replies, by kind.
type Tokens struct {
	Input, Output, CacheRead, CacheWrite5m, CacheWrite1h int64
}

// Add returns the sum of t and u.
func (t Tokens) Add(u Tokens) Tokens {
	return Tokens{t.Input + u.Input, t.Output + u.Output, t.CacheRead + u.CacheRead,
		t.CacheWrite5m + u.CacheWrite5m, t.CacheWrite1h + u.CacheWrite1h}
}

// rates are dollars per million tokens, in the order of Tokens' fields.
type rates [5]float64

// Prices per million tokens. Source: the claude-api skill's model notes
// (shared/models.md, shared/model-migration.md), 2026-11.
var (
	cards = map[string]rates{
		"claude-opus-5-5":   {4, 20, 0.20, 5, 8},
		"claude-sonnet-5-5": {2, 10, 0.20, 2.50, 4},
	}
	// Haiku 5.5 has two cards, chosen by the prompt's length.
	haiku      = rates{0.10, 0.50, 0.01, 0.125, 0.20}
	haikuLong  = rates{0.50, 2.50, 0.05, 0.625, 1}
	haikuLimit = int64(100_000)
)

// Cost prices one reply at its model. ok is false for an unknown model.
func Cost(model string, t Tokens) (usd float64, ok bool) {
	r, ok := cards[model]
	if model == "claude-haiku-5-5" {
		r, ok = haiku, true
		if t.Input+t.CacheRead+t.CacheWrite5m+t.CacheWrite1h > haikuLimit {
			r = haikuLong
		}
	}
	if !ok {
		return 0, false
	}
	n := [5]int64{t.Input, t.Output, t.CacheRead, t.CacheWrite5m, t.CacheWrite1h}
	for i := range n {
		usd += float64(n[i]) * r[i] / 1e6
	}
	return usd, true
}
