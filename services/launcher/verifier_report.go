/*
Supports the generation of text-based reports for the artifacts verify command.

This file provides an interface and implementation for creating verification reports.
It currently only supports the JSON format, but is designed to be extensible for future formats.
*/

package launcher

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"sort"
	"strings"
	"time"

	"www.velocidex.com/golang/velociraptor/config"
)

// VerifierReporter is the base interface for generating verification reports.
type VerifierReporter interface {
	AddArtifact(artifactName string, artifactPath string, state *AnalysisState)
	SetExit(err error)
	Generate(w io.Writer) error
}

// NewVerifierReporter creates a new VerifierReporter based on the specified format.
func NewVerifierReporter(format string) (VerifierReporter, error) {
	switch format {
	case "json":
		return &JsonVerifierReporter{}, nil
	default:
		return nil, fmt.Errorf("unknown report format '%v'", format)
	}
}

/*
 JSON Verification Report
*/

// JsonVerifierReporter generates a JSON formatted verification report.
type JsonVerifierReporter struct {
	artifacts []*JsonVerifierState
	exitCode  int
}

// AddArtifact adds an artifact and its analysis state to the report.
func (r *JsonVerifierReporter) AddArtifact(artifactName string, artifactPath string, state *AnalysisState) {
	r.artifacts = append(r.artifacts, &JsonVerifierState{
		Name:  artifactName,
		Path:  artifactPath,
		State: state,
	})
}

// Generate writes the JSON report to the provided writer.
func (r *JsonVerifierReporter) Generate(w io.Writer) error {
	version := config.GetVersion()

	report := &JsonVerifierFormat{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		ExitCode:  r.exitCode,
		Metadata: &VerifierMetadata{
			Version:   version.Version,
			Commit:    version.Commit,
			BuildTime: version.BuildTime,
			Command:   strings.Join(os.Args, " "),
		},
		Summary:   r.populateSummary(),
		Artifacts: r.populateArtifacts(),
		Results:   r.populateResults(),
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")

	return encoder.Encode(report)
}

// SetExit sets the exit code value for the report.
func (r *JsonVerifierReporter) SetExit(err error) {
	if err != nil {
		r.exitCode = 1
	} else {
		r.exitCode = 0
	}
}

// JsonVerifierState represents the verification state of a single artifact.
type JsonVerifierState struct {
	Name  string
	Path  string
	State *AnalysisState
}

// FormatResult formats the verification result for JSON output.
func (r *JsonVerifierState) FormatResult() *JsonVerifierResult {
	result := &JsonVerifierResult{
		Name:     r.Name,
		Path:     r.Path,
		Status:   string(r.getStatus()),
		Errors:   make([]string, 0, len(r.State.Errors)),
		Warnings: make([]string, 0, len(r.State.Warnings)),
	}

	for _, err := range r.State.Errors {
		result.Errors = append(result.Errors, err.Error())
	}

	for _, warning := range r.State.Warnings {
		result.Warnings = append(result.Warnings, warning)
	}

	return result
}

// resultStatus represents the status of a verification result.
type resultStatus string

const (
	StatusPass    resultStatus = "pass"
	StatusWarning resultStatus = "warning"
	StatusFail    resultStatus = "fail"
)

// getStatus determines the overall status of the artifact based on its errors and warnings.
func (r *JsonVerifierState) getStatus() resultStatus {
	if len(r.State.Errors) > 0 {
		return StatusFail
	}

	if len(r.State.Warnings) > 0 {
		return StatusWarning
	}

	return StatusPass
}

// JsonVerifierFormat represents the overall JSON report structure.
type JsonVerifierFormat struct {
	Timestamp string                `json:"timestamp"`
	Metadata  *VerifierMetadata     `json:"metadata"`
	ExitCode  int                   `json:"exit_code"`
	Summary   *JsonVerifierSummary  `json:"summary"`
	Artifacts []string              `json:"artifacts"`
	Results   []*JsonVerifierResult `json:"results"`
}

// VerifierMetadata contains metadata about the verification run.
type VerifierMetadata struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
	Command   string `json:"command"`
}

// JsonVerifierSummary provides a summary of the verification results.
type JsonVerifierSummary struct {
	Total   int `json:"total"`
	Passed  int `json:"passed"`
	Warning int `json:"warning"`
	Failed  int `json:"failed"`
}

// JsonVerifierResult represents the verification result for a single artifact.
type JsonVerifierResult struct {
	Name     string   `json:"name"`
	Path     string   `json:"path"`
	Status   string   `json:"status"`
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
}

// populateSummary compiles the summary of verification results.
func (r *JsonVerifierReporter) populateSummary() *JsonVerifierSummary {
	summary := &JsonVerifierSummary{}
	summary.Total = len(r.artifacts)

	for _, artifact := range r.artifacts {
		status := artifact.getStatus()
		switch status {
		case StatusPass:
			summary.Passed++
		case StatusWarning:
			summary.Warning++
		case StatusFail:
			summary.Failed++
		}
	}

	return summary
}

// populateArtifacts compiles a list of artifact names included in the report, sorted alphabetically.
func (r *JsonVerifierReporter) populateArtifacts() []string {
	artifacts := make([]string, 0, len(r.artifacts))
	seen := make(map[string]struct{}, len(r.artifacts))

	for _, artifact := range r.artifacts {
		if _, ok := seen[artifact.Name]; ok {
			continue
		}

		seen[artifact.Name] = struct{}{}
		artifacts = append(artifacts, artifact.Name)
	}

	sorted := slices.Clone(artifacts)
	sort.Strings(sorted)

	return sorted
}

// populateResults compiles the results for all artifacts, sorted by artifact path.
func (r *JsonVerifierReporter) populateResults() []*JsonVerifierResult {
	results := make([]*JsonVerifierResult, 0, len(r.artifacts))

	sortedArtifacts := slices.Clone(r.artifacts)
	slices.SortFunc(sortedArtifacts, func(a, b *JsonVerifierState) int {
		return strings.Compare(a.Path, b.Path)
	})

	for _, artifact := range sortedArtifacts {
		results = append(results, artifact.FormatResult())
	}

	return results
}
