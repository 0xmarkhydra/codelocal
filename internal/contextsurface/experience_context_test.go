package contextsurface

import (
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/projectbrain"
)

func TestCompileWithExperiencesIncludesOnlyPromotedCompatibleKnowledge(t *testing.T){
	decisions:=[]projectbrain.ExperienceDecision{
		{Status:projectbrain.ExperiencePromoted,Trust:"verified",Candidate:projectbrain.ExperienceCandidate{ID:"good",Kind:projectbrain.ExperienceWorkflow,Statement:"Run the focused package test first",BranchScope:"main",Confidence:.95,EvidenceRefs:[]string{"e1"},VerificationRefs:[]string{"v1"}}},
		{Status:projectbrain.ExperienceCandidateStatus,Candidate:projectbrain.ExperienceCandidate{ID:"draft",Kind:projectbrain.ExperienceFact,Statement:"Unverified guess",Confidence:.99}},
		{Status:projectbrain.ExperiencePromoted,Trust:"verified",Candidate:projectbrain.ExperienceCandidate{ID:"other",Kind:projectbrain.ExperienceFact,Statement:"Other branch fact",BranchScope:"feature-x",Confidence:.99}},
	}
	surface:=CompileWithExperiences(Input{Brain:projectbrain.ContextPacket{MutationAllowed:true}},decisions,"main","test",8,2000)
	found:=false
	for _,item:=range surface.Items{ if item.ID=="experience:good"{ found=true; if item.Trust!="verified"{t.Fatalf("experience trust lost: %+v",item)} }; if item.ID=="experience:draft" || item.ID=="experience:other"{t.Fatalf("incompatible experience leaked into context: %+v",item)} }
	if !found{t.Fatal("expected promoted main-branch experience")}
}
