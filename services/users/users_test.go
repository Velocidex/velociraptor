package users_test

import (
	"crypto/tls"
	"crypto/x509"
	"testing"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	acls_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/crypto"
	"www.velocidex.com/golang/velociraptor/file_store/test_utils"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/orgs"
	"www.velocidex.com/golang/velociraptor/services/users"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

type UserManagerTestSuite struct {
	test_utils.TestSuite
}

func (self *UserManagerTestSuite) SetupTest() {
	self.ConfigObj = self.TestSuite.LoadConfig()
	self.ConfigObj.Services.JournalService = true

	self.TestSuite.SetupTest()

	self.LoadArtifacts(`name: Server.Audit.Logs
type: SERVER_EVENT
`, `name: Server.Internal.UserManager
type: INTERNAL
`)
}

func (self *UserManagerTestSuite) makeUserWithRoles(username, org_id, role string) {
	org_manager, err := services.GetOrgManager()
	assert.NoError(self.T(), err)

	_, err = org_manager.GetOrgConfig(org_id)
	if err != nil {
		_, err = org_manager.CreateNewOrg(org_id, org_id, services.RandomNonce)
		assert.NoError(self.T(), err)
	}

	user_manager := services.GetUserManager()
	err = user_manager.SetUser(self.Sm.Ctx,
		&api_proto.VelociraptorUser{
			Name: username,
			Orgs: []*api_proto.OrgRecord{
				{Id: org_id},
			},
		})
	assert.NoError(self.T(), err)

	// Grant the user admin access in their respective orgs.
	org_config, err := org_manager.GetOrgConfig(org_id)
	assert.NoError(self.T(), err)

	err = services.GrantRoles(org_config, username, []string{role})
	assert.NoError(self.T(), err)
}

func (self *UserManagerTestSuite) makeUsers() {
	self.makeUserWithRoles("OrgAdmin", "", "administrator")

	self.makeUserWithRoles("AdminO1", "O1", "administrator")
	self.makeUserWithRoles("UserO1", "O1", "reader")

	self.makeUserWithRoles("AdminO2", "O2", "administrator")
	self.makeUserWithRoles("UserO2", "O2", "reader")
}

// The rest of the tests depend on this state being correct.  Make sure it is.
func (self *UserManagerTestSuite) TestMakeUsers() {
	self.makeUsers()

	golden := ordereddict.NewDict()
	users_manager := services.GetUserManager()
	user_record, err := users_manager.GetUser(self.Ctx, "OrgAdmin", "OrgAdmin")
	assert.NoError(self.T(), err)
	golden.Set("OrgAdmin OrgAdmin", user_record)

	user_record, err = users_manager.GetUser(self.Ctx, "OrgAdmin", "UserO1")
	assert.NoError(self.T(), err)
	golden.Set("OrgAdmin UserO1", user_record)

	user_record, err = users_manager.GetUser(self.Ctx, "AdminO1", "UserO1")
	assert.NoError(self.T(), err)
	golden.Set("AdminO1 UserO1", user_record)

	_, err = users_manager.GetUser(self.Ctx, "AdminO2", "UserO1")
	assert.ErrorContains(self.T(), err, "PermissionDenied")
	golden.Set("AdminO2 UserO1", err.Error())

	user_record, err = users_manager.GetUser(self.Ctx, "OrgAdmin", "UserO2")
	assert.NoError(self.T(), err)
	golden.Set("OrgAdmin UserO2", user_record)

	user_record, err = users_manager.GetUser(self.Ctx, "AdminO2", "UserO2")
	assert.NoError(self.T(), err)
	golden.Set("AdminO2 UserO2", user_record)

	_, err = users_manager.GetUser(self.Ctx, "AdminO1", "UserO2")
	assert.ErrorContains(self.T(), err, "PermissionDenied")
	golden.Set("AdminO1 UserO2", err.Error())

	// Check case insensitive user records
	user_record, err = users_manager.GetUser(self.Ctx, "OrgAdmin", "userO2")
	assert.NoError(self.T(), err)
	golden.Set("Case insensitive user02", user_record)

	goldie.Assert(self.T(), "TestMakeUsers", json.MustMarshalIndent(golden))
}

func (self *UserManagerTestSuite) TestACLS() {
	bundle, err := crypto.GenerateServerCert(self.ConfigObj, "TestUser")
	assert.NoError(self.T(), err)

	// The peer is taken from the TLS connection - the peer
	// certificate is the username that we trust.
	peer_info := &peer.Peer{
		AuthInfo: credentials.TLSInfo{
			State: tls.ConnectionState{
				PeerCertificates: []*x509.Certificate{
					bundle.Certificate,
				},
			},
		},
	}

	// Build a mocked context to emulate the one the grpc server builds.
	CA_Pool := x509.NewCertPool()
	CA_Pool.AppendCertsFromPEM([]byte(self.ConfigObj.Client.CaCertificate))

	ctx := peer.NewContext(self.Ctx, peer_info)
	user, err := users.GetGRPCUserInfo(self.ConfigObj, ctx, CA_Pool)
	assert.NoError(self.T(), err)

	assert.Equal(self.T(), "TestUser", user.Name)

	// The RPC call may impersonate another user by adding it to the
	// gRPC metadata.
	md := metadata.New(map[string]string{
		constants.GRPC_MD_USER: "AnotherUser",
	})

	ctx = metadata.NewIncomingContext(ctx, md)

	// This time impersonation is not allowed.
	user, err = users.GetGRPCUserInfo(self.ConfigObj, ctx, CA_Pool)
	assert.Error(self.T(), err)
	assert.ErrorContains(self.T(), err, "User TestUser can not impersonate AnotherUser")

	// Grant the user the impersonate permission. To be able to
	// impersonate over the API the user needs all these permissions:
	// 1. They must be a member of the org (READ_RESULTS)
	// 2. They must have the impersonate permissions (IMPERSONATE)
	// 3. They must be allowed to call over the API (API_QUERY)
	user_manager := services.GetUserManager()
	err = user_manager.AddUserToOrg(self.Ctx, services.AddNewUser,
		utils.GetSuperuserName(self.ConfigObj),
		"TestUser", []string{"root"}, &acls_proto.ApiClientACL{
			Impersonation: true,
			ReadResults:   true,
			AnyQuery:      true,
		})
	assert.NoError(self.T(), err)

	// This time impersonation is allowed.
	user, err = users.GetGRPCUserInfo(self.ConfigObj, ctx, CA_Pool)
	assert.NoError(self.T(), err)

	// The result carries the impersonated username.
	assert.Equal(self.T(), "AnotherUser", user.Name)
}

func TestUserManger(t *testing.T) {
	orgs.NonceForTest = "Nonce"

	suite.Run(t, &UserManagerTestSuite{})
}
