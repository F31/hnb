package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"
)

type TestResult struct {
	TestName string `json:"test_name"`
	Category string `json:"category"`
	Passed   bool   `json:"passed"`
	Duration string `json:"duration"`
	Error    string `json:"error,omitempty"`
}

type Report struct {
	ProviderID       string       `json:"provider_id"`
	ProviderURL      string       `json:"provider_url"`
	StartedAt        string       `json:"started_at"`
	CompletedAt      string       `json:"completed_at"`
	TotalTests       int          `json:"total_tests"`
	PassedTests      int          `json:"passed_tests"`
	FailedTests      int          `json:"failed_tests"`
	Results          []TestResult `json:"results"`
	ConformanceLevel string       `json:"conformance_level"`
}

func main() {
	providerID := flag.String("provider", "", "Provider ID to test")
	providerURL := flag.String("url", "", "Provider endpoint URL")
	manifestPath := flag.String("manifest", "", "Path to provider manifest JSON")
	outputPath := flag.String("output", "", "Output path for JSON report (default: stdout)")
	runContract := flag.Bool("contract", true, "Run contract tests")
	runFunctional := flag.Bool("functional", true, "Run functional tests")
	runFault := flag.Bool("fault", true, "Run fault injection tests")
	runSecurity := flag.Bool("security", true, "Run security tests")
	runPerformance := flag.Bool("performance", false, "Run performance baseline tests")
	storageMatrix := flag.String("storage-matrix", "", "Path to a versioned storage conformance matrix")
	storageEvidence := flag.String("storage-evidence", "", "Path to storage conformance evidence")
	flag.Parse()

	storageMode := *storageMatrix != "" || *storageEvidence != ""
	if storageMode && (*storageMatrix == "" || *storageEvidence == "") {
		log.Fatal("storage-matrix and storage-evidence must be provided together")
	}
	if !storageMode && (*providerID == "" || *providerURL == "") {
		fmt.Fprintf(os.Stderr, "Usage: provider-conformance -provider <id> -url <endpoint> [-manifest <path>] [-output <path>]\n")
		fmt.Fprintf(os.Stderr, "   or: provider-conformance -storage-matrix <path> -storage-evidence <path> [-output <path>]\n")
		flag.PrintDefaults()
		os.Exit(1)
	}

	startedAt := time.Now().UTC()
	runner := &TestRunner{
		ProviderID:  *providerID,
		ProviderURL: *providerURL,
	}

	var manifest map[string]any
	if *manifestPath != "" {
		data, err := os.ReadFile(*manifestPath)
		if err != nil {
			log.Fatalf("read manifest: %v", err)
		}
		if err := json.Unmarshal(data, &manifest); err != nil {
			log.Fatalf("parse manifest: %v", err)
		}
	}

	var results []TestResult

	if storageMode {
		storageResults, err := RunStorageMatrix(*storageMatrix, *storageEvidence)
		if err != nil {
			log.Fatal(err)
		}
		results = append(results, storageResults...)
	} else {
		if *runContract {
			results = append(results, runner.RunContractTests(manifest)...)
		}
		if *runFunctional {
			results = append(results, runner.RunFunctionalTests()...)
		}
		if *runFault {
			results = append(results, runner.RunFaultTests()...)
		}
		if *runSecurity {
			results = append(results, runner.RunSecurityTests()...)
		}
		if *runPerformance {
			results = append(results, runner.RunPerformanceBaseline()...)
		}
	}

	completedAt := time.Now().UTC()
	total := len(results)
	passed := 0
	failed := 0
	for _, r := range results {
		if r.Passed {
			passed++
		} else {
			failed++
		}
	}

	conformanceLevel := "none"
	if !storageMode && failed == 0 && total >= 10 {
		conformanceLevel = "production_ready"
	} else if !storageMode && failed == 0 {
		conformanceLevel = "basic"
	}

	report := Report{
		ProviderID:       *providerID,
		ProviderURL:      *providerURL,
		StartedAt:        startedAt.Format(time.RFC3339),
		CompletedAt:      completedAt.Format(time.RFC3339),
		TotalTests:       total,
		PassedTests:      passed,
		FailedTests:      failed,
		Results:          results,
		ConformanceLevel: conformanceLevel,
	}

	output, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		log.Fatalf("marshal report: %v", err)
	}

	if *outputPath != "" {
		if err := os.WriteFile(*outputPath, output, 0644); err != nil {
			log.Fatalf("write report: %v", err)
		}
		fmt.Printf("Report written to %s\n", *outputPath)
	} else {
		fmt.Println(string(output))
	}

	fmt.Printf("\nConformance: %s (%d/%d passed)\n", conformanceLevel, passed, total)
}
