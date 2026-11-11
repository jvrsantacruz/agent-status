// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Javier Santacruz

package price

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCost(t *testing.T) {
	t.Parallel()
	million := Tokens{1e6, 1e6, 1e6, 1e6, 1e6}
	cases := []struct {
		model string
		t     Tokens
		want  float64
		ok    bool
	}{
		{"claude-opus-5-5", million, 4 + 20 + 0.20 + 5 + 8, true},
		{"claude-opus-5-5", Tokens{Input: 2, Output: 203, CacheWrite5m: 37765}, (2*4 + 203*20 + 37765*5) / 1e6, true},
		{"claude-sonnet-5-5", million, 2 + 10 + 0.20 + 2.50 + 4, true},
		{"claude-haiku-5-5", Tokens{Input: 1000, Output: 1000}, (1000*0.10 + 1000*0.50) / 1e6, true},
		{"claude-haiku-5-5", Tokens{Input: 1000, CacheRead: 100_000, Output: 1000}, (1000*0.50 + 100_000*0.05 + 1000*2.50) / 1e6, true},
		{"deepseek-ai/DeepSeek-V4-Pro", million, 0, false},
		{"<synthetic>", million, 0, false},
	}
	for _, c := range cases {
		got, ok := Cost(c.model, c.t)
		assert.Equal(t, c.ok, ok, c.model)
		assert.InDelta(t, c.want, got, 1e-9, c.model)
	}
}
