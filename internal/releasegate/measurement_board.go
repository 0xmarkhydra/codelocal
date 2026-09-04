package releasegate

// BoardSignals carries measurements that task samples do not capture on their
// own: cost, call volumes, waste ratios and lane verification rates.
type BoardSignals struct {
	CostPerVerifiedTask     float64 `json:"costPerVerifiedTask"`
	TimePerVerifiedTaskMs   int64   `json:"timePerVerifiedTaskMs"`
	ModelCallsPerTask       float64 `json:"modelCallsPerTask"`
	ToolCallsPerTask        float64 `json:"toolCallsPerTask"`
	RetryWaste              int64   `json:"retryWaste"`
	DuplicateContextRatio   float64 `json:"duplicateContextRatio"`
	ContextCacheHitRate     float64 `json:"contextCacheHitRate"`
	BrainReuseRate          float64 `json:"brainReuseRate"`
	BrowserVerificationRate float64 `json:"browserVerificationRate"`
	MobileVerificationRate  float64 `json:"mobileVerificationRate"`
}

// MeasurementBoard is the plan X tracking contract. Rate math comes from
// AggregateSamples so the board can never disagree with the release gate.
type MeasurementBoard struct {
	VerifiedSuccessRate     float64 `json:"verifiedSuccessRate"`
	TokensPerVerifiedTask   int64   `json:"tokensPerVerifiedTask"`
	CostPerVerifiedTask     float64 `json:"costPerVerifiedTask"`
	TimePerVerifiedTaskMs   int64   `json:"timePerVerifiedTaskMs"`
	ModelCallsPerTask       float64 `json:"modelCallsPerTask"`
	ToolCallsPerTask        float64 `json:"toolCallsPerTask"`
	RetryWaste              int64   `json:"retryWaste"`
	DuplicateContextRatio   float64 `json:"duplicateContextRatio"`
	ContextCacheHitRate     float64 `json:"contextCacheHitRate"`
	BrainReuseRate          float64 `json:"brainReuseRate"`
	HumanInterventions      float64 `json:"humanInterventionsPerTask"`
	ResumeSuccessRate       float64 `json:"resumeSuccessRate"`
	LostUpdateCount         int64   `json:"lostUpdateCount"`
	SecurityRegressionCount int64   `json:"securityRegressionCount"`
	BrowserVerificationRate float64 `json:"browserVerificationRate"`
	MobileVerificationRate  float64 `json:"mobileVerificationRate"`
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
