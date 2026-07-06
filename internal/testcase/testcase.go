package testcase

import (
	"sync"

	"go.uber.org/zap"

	"pipet/internal/httpclient"
	"pipet/internal/logger"
)

type TestCase struct {
	Name       string
	Run        func() error
	Skip       bool
	SkipReason string
}

var testCases []TestCase
var mu sync.Mutex

func RegisterTest(name string, run func() error) {
	mu.Lock()
	defer mu.Unlock()
	testCases = append(testCases, TestCase{
		Name: name,
		Run:  run,
	})
}

func RegisterSkippedTest(name string, reason string) {
	mu.Lock()
	defer mu.Unlock()
	testCases = append(testCases, TestCase{
		Name:       name,
		Skip:       true,
		SkipReason: reason,
	})
}

func RunAll() {
	httpclient.InitClient()

	logger.Info("Starting API tests...")

	var wg sync.WaitGroup
	results := make(chan testResult, len(testCases))

	for _, tc := range testCases {
		wg.Add(1)
		go func(tc TestCase) {
			defer wg.Done()
			if tc.Skip {
				logger.Info("Skipping test", zap.String("name", tc.Name), zap.String("reason", tc.SkipReason))
				results <- testResult{name: tc.Name, skipped: true, skipReason: tc.SkipReason}
				return
			}

			logger.Info("Running test", zap.String("name", tc.Name))
			err := tc.Run()
			if err != nil {
				logger.Error("Test failed", zap.String("name", tc.Name), zap.Error(err))
				results <- testResult{name: tc.Name, failed: true, err: err}
			} else {
				logger.Info("Test passed", zap.String("name", tc.Name))
				results <- testResult{name: tc.Name, passed: true}
			}
		}(tc)
	}

	wg.Wait()
	close(results)

	summarize(results)
}

type testResult struct {
	name       string
	passed     bool
	failed     bool
	skipped    bool
	skipReason string
	err        error
}

func summarize(results <-chan testResult) {
	var passed, failed, skipped int

	for r := range results {
		if r.passed {
			passed++
		} else if r.failed {
			failed++
		} else if r.skipped {
			skipped++
		}
	}

	logger.Info("Test summary",
		zap.Int("total", passed+failed+skipped),
		zap.Int("passed", passed),
		zap.Int("failed", failed),
		zap.Int("skipped", skipped))

	if failed > 0 {
		logger.Error("Some tests failed")
	} else {
		logger.Info("All tests passed!")
	}
}
