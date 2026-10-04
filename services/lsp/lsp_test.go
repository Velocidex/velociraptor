package lsp_test

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
)

type LSPTestSuite struct {
	test_utils.TestSuite
}

func TestLSPServer(t *testing.T) {
	suite.Run(t, &LSPTestSuite{})
}
