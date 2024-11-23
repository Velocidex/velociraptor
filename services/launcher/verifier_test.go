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
			"Call to Artifact.Generic.Client.Info contain unknown parameter Foo"},

		{"Calling unknown Artifact",
			"SELECT * FROM Artifact.Generic.Unknown.Artifact()",
			"Query calls Unknown artifact Generic.Unknown.Artifact"},

		// No errors - all good
		{"Function with args",
			`SELECT parse_string_with_regex(string='hello', regex='bar') FROM scope()`, ""},

		{"Function with args - Missing required arg",
			`SELECT parse_string_with_regex(string='hello') FROM scope()`,
			"While calling vql function parse_string_with_regex.+, required arg regex is not called"},
	}
)

func (self *LauncherTestSuite) TestVerifyVQL() {
	manager, err := services.GetRepositoryManager(self.ConfigObj)
	assert.NoError(self.T(), err)

	repository, err := manager.GetGlobalRepository(self.ConfigObj)
	assert.NoError(self.T(), err)

	for _, tc := range verifier_test_cases {
		errs := launcher.VerifyVQL(self.Ctx,
			self.ConfigObj, tc.query, repository)
		if len(errs) > 0 {
			if tc.error_regex == "" {
				self.T().Fatalf("%v: Expected no error but got %v",
					tc.desc, errs)
			}

			assert.Regexp(self.T(), tc.error_regex, fmt.Sprintf("%v", errs))
		}
	}
}
