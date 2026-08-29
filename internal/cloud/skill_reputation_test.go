package cloud

import "testing"

func TestBayesianRatingQualityUsesConservativePrior(t *testing.T) {
	if got := bayesianRatingQuality(0, 0); got != 0.7 {
		t.Fatalf("no-rating prior=%v want 0.7", got)
	}
	oneFiveStar := bayesianRatingQuality(5, 1)
	if oneFiveStar <= 0.7 || oneFiveStar >= 1 {
		t.Fatalf("one rating must improve quality without dominating, got %.3f", oneFiveStar)
	}
	manyFiveStar := bayesianRatingQuality(5, 100)
	if manyFiveStar <= oneFiveStar || manyFiveStar > 1 {
		t.Fatalf("large evidence should outweigh prior, one=%.3f many=%.3f", oneFiveStar, manyFiveStar)
	}
}

func TestClampSkillScoreBoundsRankingSignal(t *testing.T) {
	if clampSkillScore(-1) != 0 || clampSkillScore(2) != 1 || clampSkillScore(0.42) != 0.42 {
		t.Fatal("skill quality clamp contract changed")
	}
}
