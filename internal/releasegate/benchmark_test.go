package releasegate

import "testing"

func TestAggregateSamplesCountsOnlyVerifiedTaskTokens(t *testing.T){
	metrics,err:=AggregateSamples([]TaskSample{{TaskID:"a",Verified:true,Tokens:100,LatencyMillis:10,ResumeAttempted:true,ResumeSucceeded:true},{TaskID:"b",Verified:false,Tokens:1,LatencyMillis:30,HumanInterventions:2,ResumeAttempted:true,ResumeSucceeded:false}})
	if err!=nil{t.Fatal(err)}
	if metrics.VerifiedSuccessRate!=.5{t.Fatalf("success rate=%v",metrics.VerifiedSuccessRate)}
	if metrics.MedianTokensPerVerifiedTask!=100{t.Fatalf("failed cheap task polluted token metric: %d",metrics.MedianTokensPerVerifiedTask)}
	if metrics.ResumeSuccessRate!=.5{t.Fatalf("resume rate=%v",metrics.ResumeSuccessRate)}
	if metrics.HumanInterventionsPerTask!=1{t.Fatalf("interventions=%v",metrics.HumanInterventionsPerTask)}
}
