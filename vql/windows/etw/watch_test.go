package etw_test

import (
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/velociraptor/vql/windows/etw"
)

func TestSession(t *testing.T) {
	suite.Run(t, new(sessionSuite))
}

type sessionSuite struct {
	test_utils.TestSuite
}

func (self *sessionSuite) SetupTest() {
	self.ConfigObj = self.LoadConfig()
	self.TestSuite.SetupTest()

	self.LoadArtifactFiles(
		"../../../artifacts/definitions/Demo/Plugins/GUI.yaml",
		"../../../artifacts/definitions/Reporting/Default.yaml",
	)

	Clock := utils.NewMockClock(time.Unix(1602103388, 0))
	reporting.Clock = Clock
}

func (self *sessionSuite) TestSession_KernelProcess() {
	closer := utils.MockTime(utils.NewMockClock(time.Unix(10, 10)))
	defer closer()

	manager, _ := services.GetRepositoryManager(self.ConfigObj)

	// Now create a download of this collection.
	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: acl_managers.NullACLManager{},
		Logger:     logging.NewPlainLogger(self.ConfigObj, &logging.FrontendComponent),
		Env:        ordereddict.NewDict(),
	}

	ctx := self.Ctx
	scope := manager.BuildScope(builder)

	// Test with valid GUID for Microsoft-Windows-Kernel-Process
	validArgs := ordereddict.NewDict().
		Set("guid", "{22FB2CD6-0E7B-422B-A0C7-2FAD1FD0E716}"). // GUID for Microsoft-Windows-Kernel-Process
		Set("any", 0x50)                                       // Process and Image loads
	output := etw.WatchETWPlugin{}.Call(ctx, scope, validArgs)

	// Test that we can receive a large amount of messages without a timeout.
	msg, ok := <-output
	self.True(ok, "Output channel should not be closed for valid GUID")
	self.NotNil(msg, "Message should not be nil for valid GUID")
	scope.Log("msg: %v", msg)
}

// Test multiples GUIDs with same session
func (self *sessionSuite) TestSession_MultipleProviders() {
	closer := utils.MockTime(utils.NewMockClock(time.Unix(10, 10)))
	defer closer()

	manager, _ := services.GetRepositoryManager(self.ConfigObj)

	// Now create a download of this collection.
	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: acl_managers.NullACLManager{},
		Logger:     logging.NewPlainLogger(self.ConfigObj, &logging.FrontendComponent),
		Env:        ordereddict.NewDict(),
	}

	ctx := self.Ctx
	scope := manager.BuildScope(builder)

	// Test with valid GUID for Microsoft-Windows-Kernel-Process
	output := etw.WatchETWPlugin{}.Call(ctx, scope, ordereddict.NewDict().
		Set("guid", "{22FB2CD6-0E7B-422B-A0C7-2FAD1FD0E716}")) // GUID for Microsoft-Windows-Kernel-Process
	msg, ok := <-output
	self.True(ok, "Output channel should not be closed for valid GUID")
	scope.Log("msg1: %v", msg)

	output2 := etw.WatchETWPlugin{}.Call(ctx, scope, ordereddict.NewDict().
		Set("guid", "{ABF1F586-2E50-4BA8-928D-49044E6F0DB7}")) // GUID for Microsoft-Windows-Kernel-IO
	msg2, ok := <-output2
	self.True(ok, "Output channel should not be closed for valid GUID")
	scope.Log("msg2: %v", msg2)
	self.NotNil(msg, "Message should not be nil for valid GUID")

	msg, ok = <-output
	self.True(ok, "Output channel should not be closed for valid GUID")
	scope.Log("msg1: %v", msg)
}

func (self *sessionSuite) TestFailures() {

	closer := utils.MockTime(utils.NewMockClock(time.Unix(10, 10)))
	defer closer()

	manager, _ := services.GetRepositoryManager(self.ConfigObj)

	// Now create a download of this collection.
	builder := services.ScopeBuilder{
		Config:     self.ConfigObj,
		ACLManager: acl_managers.NullACLManager{},
		Logger:     logging.NewPlainLogger(self.ConfigObj, &logging.FrontendComponent),
		Env:        ordereddict.NewDict(),
	}

	ctx := self.Ctx
	scope := manager.BuildScope(builder)
	plugin := etw.WatchETWPlugin{}

	// Test with invalid GUID
	invalidArgs := ordereddict.NewDict().
		Set("guid", "invalidGUID")
	output := plugin.Call(ctx, scope, invalidArgs)
	_, ok := <-output
	self.False(ok, "Output channel should be closed for invalid GUID")

	// Test with valid GUID but without context
	validArgs := ordereddict.NewDict().
		Set("guid", "00000000-0000-0000-0000-000000000000")
	output = plugin.Call(ctx, scope, validArgs)
	_, ok = <-output
	self.False(ok, "Output channel should be closed when context is nil")
}
