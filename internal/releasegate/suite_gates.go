package releasegate

import (
	"errors"
	"fmt"
)

type BenchmarkSuite string

const (
	SuiteSingleAgent BenchmarkSuite = "single_agent"
	SuiteToolProgram BenchmarkSuite = "tool_program"
	SuiteMultiAgent  BenchmarkSuite = "multi_agent"

	minSuiteSamples = 5
)

var ErrBenchmarkSuiteTooSmall = errors.New("benchmark suite has too few samples")

type SuiteReport struct {
	Suite  BenchmarkSuite  "suite"
	Report BenchmarkReport "report"
}

// EvaluateSuite runs the shared release gate for one benchmark suite (plan
// gates H5, J3 and K4). Suites share the metric math but never share
// samples: each side needs a minimum sample count so a gate cannot pass on
// anecdotal evidence.
func EvaluateSuite(suite BenchmarkSuite, baselineSamples, candidateSamples []TaskSample, infra Infrastructure, chaos []ChaosResult, cfg Config) (SuiteReport, error) {
	switch suite {
	case SuiteSingleAgent, SuiteToolProgram, SuiteMultiAgent:
	default:
		return SuiteReport{}, fmt.Errorf("%w: %q", ErrBenchmarkSuiteTooSmall, string(suite))
	}
	if len(baselineSamples) < minSuiteSamples || len(candidateSamples) < minSuiteSamples {
		return SuiteReport{}, fmt.Errorf("%w: need at least %d samples per side", ErrBenchmarkSuiteTooSmall, minSuiteSamples)
	}
	report, err := EvaluateSamples(baselineSamples, candidateSamples, infra, chaos, cfg)
	if err != nil {
		return SuiteReport{}, err
	}
	return SuiteReport{Suite: suite, Report: report}, nil
}
