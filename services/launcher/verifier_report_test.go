package launcher

import (
	"bytes"
	"fmt"
	"slices"
	"testing"

	"www.velocidex.com/golang/velociraptor/json"
)

// Helper function to create an AnalysisState for testing.
func state(artifact string, errors []error, warnings []string) *AnalysisState {
	return &AnalysisState{
		Artifact:    artifact,
		Permissions: nil,
		Errors:      errors,
		Warnings:    warnings,
	}
}

func TestNewVerifierReporter(t *testing.T) {
	reporter, err := NewVerifierReporter("json")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if _, ok := reporter.(*JsonVerifierReporter); !ok {
		t.Fatalf("expected JsonVerifierReporter, got %T", reporter)
	}

	_, err = NewVerifierReporter("unknown")
	if err == nil {
		t.Fatalf("expected error for unknown format, got nil")
	}
}

func TestJsonVerifierStateGetStatus(t *testing.T) {
	tests := []struct {
		name     string
		state    *AnalysisState
		expected resultStatus
	}{
		{
			name:     "pass",
			state:    state("Artifact1", nil, nil),
			expected: StatusPass,
		},
		{
			name:     "warning",
			state:    state("Artifact2", nil, []string{"warning"}),
			expected: StatusWarning,
		},
		{
			name:     "fail",
			state:    state("Artifact3", []error{fmt.Errorf("error")}, nil),
			expected: StatusFail,
		},
		{
			name:     "fail_with_warning",
			state:    state("Artifact4", []error{fmt.Errorf("error")}, []string{"warning"}),
			expected: StatusFail,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonState := &JsonVerifierState{
				Name:  tt.state.Artifact,
				State: tt.state,
			}
			if status := jsonState.getStatus(); status != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, status)
			}
		})
	}
}

func TestJsonVerifierStateFormatResult(t *testing.T) {
	js := &JsonVerifierState{
		Name:  "Artifact1",
		Path:  "./Artifact1.yaml",
		State: state("Artifact1", []error{fmt.Errorf("error1"), fmt.Errorf("error2")}, []string{"warning1", "warning2"}),
	}

	result := js.FormatResult()

	if result.Name != "Artifact1" {
		t.Errorf("expected name 'Artifact1', got '%s'", result.Name)
	}

	if result.Path != "./Artifact1.yaml" {
		t.Errorf("expected path './Artifact1.yaml', got '%s'", result.Path)
	}

	if result.Status != string(StatusFail) {
		t.Errorf("expected status 'fail', got '%s'", result.Status)
	}

	if len(result.Errors) != 2 || result.Errors[0] != "error1" || result.Errors[1] != "error2" {
		t.Errorf("unexpected errors: %v", result.Errors)
	}

	if len(result.Warnings) != 2 || result.Warnings[0] != "warning1" || result.Warnings[1] != "warning2" {
		t.Errorf("unexpected warnings: %v", result.Warnings)
	}
}

func TestJsonVerifierPopulateSummary(t *testing.T) {
	report := &JsonVerifierReporter{}
	report.AddArtifact("Artifact1", "./Artifact1.yaml", state("Artifact1", nil, nil))
	report.AddArtifact("Artifact2", "./Artifact2.yaml", state("Artifact2", []error{fmt.Errorf("error")}, nil))
	report.AddArtifact("Artifact3", "./Artifact3.yaml", state("Artifact3", nil, []string{"warning"}))
	report.AddArtifact("Artifact2", "./Artifact2.yaml", state("Artifact2", []error{fmt.Errorf("error")}, []string{"warning"}))

	result := report.populateSummary()

	if result.Total != 4 || result.Passed != 1 || result.Warning != 1 || result.Failed != 2 {
		t.Errorf("unexpected summary: %+v", result)
	}
}

func TestJsonVerifierPopulateArtifacts(t *testing.T) {
	report := &JsonVerifierReporter{}
	report.AddArtifact("Artifact2", "./Artifact2.yaml", state("Artifact2", nil, nil))
	report.AddArtifact("Artifact1", "./Artifact1.yaml", state("Artifact1", nil, nil))
	report.AddArtifact("Artifact3", "./Artifact3.yaml", state("Artifact3", nil, nil))

	artifacts := report.populateArtifacts()
	expected := []string{"Artifact1", "Artifact2", "Artifact3"}

	if !slices.Equal(artifacts, expected) {
		t.Errorf("expected %v, got %v", expected, artifacts)
	}
}
func TestJsonVerifierPopulateResults(t *testing.T) {
	report := &JsonVerifierReporter{}
	report.AddArtifact("Artifact1", "./Artifact1.yaml", state("Artifact1", nil, nil))
	report.AddArtifact("Artifact2", "./Artifact2.yaml", state("Artifact2", nil, nil))

	result := report.populateResults()

	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}

	if result[0].Path != "./Artifact1.yaml" || result[1].Path != "./Artifact2.yaml" {
		t.Errorf("results not sorted by path: %v", result)
	}
}

func TestJsonVerifierGenerate(t *testing.T) {
	report := &JsonVerifierReporter{}
	report.AddArtifact("Artifact1", "./Artifact1.yaml", state("Artifact1", nil, nil))
	report.SetExit(nil)

	var buf bytes.Buffer
	if err := report.Generate(&buf); err != nil {
		t.Fatalf("generate failed: %v", err)
	}

	var result JsonVerifierFormat
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}

	if result.Summary.Passed != 1 {
		t.Errorf("expected 1 passed, got %d", result.Summary.Passed)
	}

	if len(result.Results) != 1 || result.Results[0].Status != "pass" {
		t.Errorf("unexpected results: %v", result.Results)
	}
}
