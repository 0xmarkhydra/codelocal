package releasegate

// BoardSignals carries measurements that task samples do not capture on their
// own: cost, call volumes, waste ratios and lane verification rates.
type BoardSignals struct {
	CostPerVerifiedTask     float64 "costPerVerifiedTask"
	TimePerVerifiedTaskMs   int64   "timePerVerifiedTaskMs"
	ModelCallsPerTask       float64 "modelCallsPerTask"
	ToolCallsPerTask        float64 "toolCallsPerTask"
	RetryWaste              int64   "retryWaste"
	DuplicateContextRatio   float64 "duplicateContextRatio"
	ContextCacheHitRate     float64 "contextCacheHitRate"
	BrainReuseRate          float64 "brainReuseRate"
	BrowserVerificationRate float64 "browserVerificationRate"
	MobileVerificationRate  float64 "mobileVerificationRate"
}

// MeasurementBoard is the plan X tracking contract. Rate math comes from
// AggregateSamples so the board can never disagree with the release gate.
type MeasurementBoard struct {
	VerifiedSuccessRate     float64 "verifiedSuccessRate"
	TokensPerVerifiedTask   int64   "tokensPerVerifiedTask"
	CostPerVerifiedTask     float64 "costPerVerifiedTask"
	TimePerVerifiedTaskMs   int64   "timePerVerifiedTaskMs"
	ModelCallsPerTask       float64 "modelCallsPerTask"
	ToolCallsPerTask        float64 "toolCallsPerTask"
	RetryWaste              int64   "retryWaste"
	DuplicateContextRatio   float64 "duplicateContextRatio"
	ContextCacheHitRate     float64 "contextCacheHitRate"
	BrainReuseRate          float64 "brainReuseRate"
	HumanInterventions      float64 "humanInterventionsPerTask"
	ResumeSuccessRate       float64 "resumeSuccessRate"
	LostUpdateCount         int64   "lostUpdateCount"
	SecurityRegressionCount int64   "securityRegressionCount"
	BrowserVerificationRate float64 "browserVerificationRate"
	MobileVerificationRate  float64 "mobileVerificationRate"
}

func SummarizeBoard(samples []TaskSample, signals BoardSignals) (MeasurementBoard, error) {
	metrics, err := AggregateSamples(samples)
	if err != nil {
		return MeasurementBoard{}, err
	}
	return MeasurementBoard{
		VerifiedSuccessRate:     metrics.VerifiedSuccessRate,
		TokensPerVerifiedTask:   metrics.MedianTokensPerVerifiedTask,
		CostPerVerifiedTask:     signals.CostPerVerifiedTask,
		TimePerVerifiedTaskMs:   signals.TimePerVerifiedTaskMs,
		ModelCallsPerTask:       signals.ModelCallsPerTask,
		ToolCallsPerTask:        signals.ToolCallsPerTask,
		RetryWaste:              signals.RetryWaste,
		DuplicateContextRatio:   signals.DuplicateContextRatio,
		ContextCacheHitRate:     signals.ContextCacheHitRate,
		BrainReuseRate:          signals.BrainReuseRate,
		HumanInterventions:      metrics.HumanInterventionsPerTask,
		ResumeSuccessRate:       metrics.ResumeSuccessRate,
		LostUpdateCount:         metrics.LostUpdates,
		SecurityRegressionCount: metrics.SecurityRegressions,
		BrowserVerificationRate: signals.BrowserVerificationRate,
		MobileVerificationRate:  signals.MobileVerificationRate,
	}, nil
}
