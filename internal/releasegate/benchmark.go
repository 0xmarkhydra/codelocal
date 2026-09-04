package releasegate

import (
	"errors"
	"sort"
)

var ErrNoBenchmarkSamples = errors.New("no benchmark samples")

type TaskSample struct {
	TaskID             string `json:"taskId"`
	Verified           bool   `json:"verified"`
	Tokens             int64  `json:"tokens"`
	LatencyMillis      int64  `json:"latencyMillis"`
	HumanInterventions int64  `json:"humanInterventions"`
	ResumeAttempted    bool   `json:"resumeAttempted"`
	ResumeSucceeded    bool   `json:"resumeSucceeded"`
	SecurityRegressions int64 `json:"securityRegressions"`
	LostUpdates        int64  `json:"lostUpdates"`
	PolicyBypasses     int64  `json:"policyBypasses"`
}

type BenchmarkReport struct {
	Baseline  Metrics `json:"baseline"`
	Candidate Metrics `json:"candidate"`
	Gate      Gate    `json:"gate"`
}

// AggregateSamples converts task-level benchmark evidence into the exact metric
// shape consumed by the production release gate. Token efficiency is measured
// only on verified tasks so a failed cheap run can never improve the score.
func AggregateSamples(samples []TaskSample) (Metrics, error) {
	if len(samples)==0{return Metrics{},ErrNoBenchmarkSamples}
	verified:=0
	verifiedTokens:=[]int64{}
	latencies:=make([]int64,0,len(samples))
	resumeAttempts,resumeSuccess:=0,0
	var interventions int64
	metrics:=Metrics{}
	for _,sample:=range samples{
		if sample.Verified{verified++;if sample.Tokens>0{verifiedTokens=append(verifiedTokens,sample.Tokens)}}
		if sample.LatencyMillis>=0{latencies=append(latencies,sample.LatencyMillis)}
		interventions+=nonNegativeSample(sample.HumanInterventions)
		if sample.ResumeAttempted{resumeAttempts++;if sample.ResumeSucceeded{resumeSuccess++}}
		metrics.SecurityRegressions+=nonNegativeSample(sample.SecurityRegressions)
		metrics.LostUpdates+=nonNegativeSample(sample.LostUpdates)
		metrics.PolicyBypasses+=nonNegativeSample(sample.PolicyBypasses)
	}
	metrics.VerifiedSuccessRate=float64(verified)/float64(len(samples))
	metrics.HumanInterventionsPerTask=float64(interventions)/float64(len(samples))
	if resumeAttempts>0{metrics.ResumeSuccessRate=float64(resumeSuccess)/float64(resumeAttempts)}
	metrics.MedianTokensPerVerifiedTask=percentileInt64(verifiedTokens,.5)
	metrics.P95LatencyMillis=percentileInt64(latencies,.95)
	return metrics,nil
}

func EvaluateSamples(baselineSamples,candidateSamples []TaskSample,infra Infrastructure,chaos []ChaosResult,cfg Config)(BenchmarkReport,error){
	baseline,err:=AggregateSamples(baselineSamples);if err!=nil{return BenchmarkReport{},err}
	candidate,err:=AggregateSamples(candidateSamples);if err!=nil{return BenchmarkReport{},err}
	return BenchmarkReport{Baseline:baseline,Candidate:candidate,Gate:Evaluate(baseline,candidate,infra,chaos,cfg)},nil
}

func percentileInt64(values []int64,p float64)int64{
	if len(values)==0{return 0}
	copyValues:=append([]int64(nil),values...);sort.Slice(copyValues,func(i,j int)bool{return copyValues[i]<copyValues[j]})
	if p<=0{return copyValues[0]};if p>=1{return copyValues[len(copyValues)-1]}
	index:=int(float64(len(copyValues)-1)*p+.5);if index<0{index=0};if index>=len(copyValues){index=len(copyValues)-1};return copyValues[index]
}
func nonNegativeSample(value int64)int64{if value<0{return 0};return value}
