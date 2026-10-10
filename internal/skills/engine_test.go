package skills

import (
	"testing"
)

func TestCalculateTokenSavingsSingleSkills(t *testing.T) {
	cases := []struct {
		skill    string
		output   int
		minSaved float64
		maxSaved float64
	}{
		{"caveman", 100, 55.0, 68.0},      // ~62%
		{"ponytail", 100, 42.0, 55.0},     // ~48%
		{"rtk", 100, 32.0, 45.0},          // ~38%
		{"context_mode", 100, 26.0, 38.0}, // ~32%
		{"codegraph", 100, 22.0, 34.0},    // ~28%
	}

	for _, tc := range cases {
		_, _, savedPct, hasSaver := CalculateTokenSavings(tc.output, []string{tc.skill})
		if !hasSaver {
			t.Errorf("expected hasSaver=true for skill %s", tc.skill)
		}
		if savedPct < tc.minSaved || savedPct > tc.maxSaved {
			t.Errorf("skill %s: expected savings between %.1f%% and %.1f%%, got %.2f%%", tc.skill, tc.minSaved, tc.maxSaved, savedPct)
		}
	}
}

func TestCalculateTokenSavingsMultiSkillDiminishingReturns(t *testing.T) {
	// Single skill
	_, _, cavemanPct, _ := CalculateTokenSavings(100, []string{"caveman"})
	// Multi skill combo
	_, _, comboPct, _ := CalculateTokenSavings(100, []string{"caveman", "ponytail"})

	if comboPct <= cavemanPct {
		t.Errorf("expected comboPct (%.2f%%) > single cavemanPct (%.2f%%)", comboPct, cavemanPct)
	}

	// 5-Skill combo capped reasonably at ~82%
	_, _, allPct, _ := CalculateTokenSavings(100, []string{"rtk", "caveman", "ponytail", "context_mode", "codegraph"})
	if allPct > 85.0 || allPct < 75.0 {
		t.Errorf("expected 5-skill combo to be ~80%%-82%%, got %.2f%%", allPct)
	}
}
