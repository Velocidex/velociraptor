package launcher_test

import (
	"fmt"

	"github.com/stretchr/testify/assert"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/launcher"
)

var (
	verifier_test_cases = []struct {
		desc        string
		query       string
		error_regex string
	}{
		{"Calling Artifact",
			"SELECT * FROM Artifact.Generic.Client.Info()", ""},

		{"Calling Artifact with unknown parameter",
			"SELECT * FROM Artifact.Generic.Client.Info(Foo=3)",
			"Call to Artifact.Generic.Client.Info contains unknown parameter Foo"},
		{"Calling Artifact with known parameter",
			"SELECT * FROM Artifact.Artifact.With.Parameters(`Param1`='hello')",
			""},

		{"Calling unknown Artifact",
			"SELECT * FROM Artifact.Generic.Unknown.Artifact()",
			"Query calls Unknown artifact Generic.Unknown.Artifact"},

		// Handling VQL definitions
		{"Calling unknown plugin",
			"SELECT * FROM infoxxxx()",
			"Unknown plugin infoxxxx()"},

		{"Define VQL function and call it",
			"LET infoxxx = SELECT * FROM info() SELECT * FROM infoxxx", ""},

		// With some parameters
		{"Define VQL function - Do not pass arg",
			"LET infoxxx(X) = SELECT * FROM info() SELECT * FROM infoxxx()",
			"While calling VQL definition infoxxx(), required arg X is not provided"},

		// With some parameters
		{"Define VQL function - Pass incorrect arg",
			"LET infoxxx(X) = SELECT * FROM info() SELECT * FROM infoxxx(X=1, Y=2)",
			"Invalid arg Y for VQL definition infoxxx"},

		// No errors - all good
		{"Function with args",
			`SELECT parse_string_with_regex(string='hello', regex='bar') FROM scope()`, ""},

		{"Function with args - Missing required arg",
			`SELECT parse_string_with_regex(string='hello') FROM scope()`,
			"While calling vql function parse_string_with_regex.+, required arg regex is not called"},

		{"Function with args - delimited by `",
			"SELECT parse_string_with_regex(`string`='hello', `regex`='bar') FROM scope()",
			""},
	}
)

func (self *LauncherTestSuite) TestVerifyVQL() {
	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	repository, err := manager.GetGlobalRepository(self.ConfigObj)
	assert.NoError(self.T(), err)

	for _, tc := range verifier_test_cases {
		state := launcher.NewAnalysisState("")
		errs := launcher.VerifyVQL(self.Ctx,
			self.ConfigObj, tc.query, repository, state)
		if len(errs) > 0 {
			if tc.error_regex == "" {
				self.T().Fatalf("%v: Expected no error but got %v",
					tc.desc, errs)
			}

			assert.Regexp(self.T(), fmt.Sprintf("%v", errs), tc.error_regex)
		}
	}
}

var (
	verifier_test_cases_artifacts = []struct {
		desc        string
		artifact    string
		error_regex string
	}{
		{"Invalid artifact precondition", `
name: Test
precondition: SELECT * FROM infox()
`, "Test: precondition: Unknown plugin infox()"},

		{"Invalid source precondition", `
name: Test
sources:
- name: Source
  precondition: SELECT * FROM infox()
`, "Test/Source: precondition: Unknown plugin infox()"},

		{"Invalid export", `
name: TestExport
export:
  SELECT * FROM infox()
`, "TestExport: export: Unknown plugin infox()"},

		{"Invalid source query", `
name: Test
sources:
- name: Source
  query: SELECT * FROM infox()
`, "Test/Source: query: Unknown plugin infox()"},

		{"Define in export, use in query", `
name: TestExport
export: |
  LET infox = SELECT * FROM info()

sources:
- name: Source
  query: SELECT * FROM infox()
`, ""},

		{"Define in export, use in import", `
name: TestImport
imports:
  - TestExport

sources:
- name: Source
  query: SELECT * FROM infox()
`, ""},

		{"Invalid import", `
name: TestImport
imports:
  - TestDoesNotExist

sources:
- name: Source
  query: SELECT * FROM infox()
`, "TestImport: invalid import: Artifact TestDoesNotExist not found"},

		{"Import artifact that does not export anything", `
name: TestImport
imports:
  - Test

sources:
- name: Source
  query: SELECT * FROM infox()
`, "TestImport: invalid import: Artifact Test does not export anything"},
	}
)

func (self *LauncherTestSuite) TestVerifyArtifact() {
	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	repository, err := manager.GetGlobalRepository(self.ConfigObj)
	assert.NoError(self.T(), err)

	for _, tc := range verifier_test_cases_artifacts {
		state := launcher.NewAnalysisState("")

		artifact, err := repository.LoadYaml(tc.artifact, services.ArtifactOptions{
			ValidateArtifact: true,
		})
		assert.NoError(self.T(), err)

		launcher.VerifyArtifact(self.Ctx, self.ConfigObj, repository, artifact, state)

		if len(state.Errors) > 0 {
			if tc.error_regex == "" {
				self.T().Fatalf("%v: Expected no error but got %v",
					tc.desc, state.Errors)
			}

			assert.Regexp(self.T(), tc.error_regex, fmt.Sprintf("%v", state.Errors))
		}
	}
}
