package jit_manager

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

var jitStorePath = path_specs.NewSafeFilestorePath("config", "jit_requests")

const (
	maxPendingPerUser  = 5
	maxJustificationLen = 1024
	maxRequestAge       = 7 * 24 * 60 * 60 // 7 days, for garbage collection
)

// Roles that cannot be requested via JIT due to their dangerous permissions.
var blockedJITRoles = []string{"reader", "api", "org_admin"}

type JITManager struct {
	mu         sync.Mutex
	config_obj *config_proto.Config
	requests   map[string]*api_proto.JITRoleRequest
}

func NewJITManager(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) (*JITManager, error) {

	self := &JITManager{
		config_obj: config_obj,
		requests:   make(map[string]*api_proto.JITRoleRequest),
	}

	self.load()

	wg.Add(1)
	go func() {
		defer wg.Done()
		self.loop(ctx)
	}()

	return self, nil
}

func (self *JITManager) loop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			self.expireAndCleanup()
		}
	}
}

func (self *JITManager) expireAndCleanup() {
	self.mu.Lock()
	defer self.mu.Unlock()

	now := time.Now().Unix()
	changed := false
	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)

	for id, r := range self.requests {
		if r.Status == api_proto.JIT_STATUS_APPROVED &&
			r.ExpiresTime > 0 && now >= r.ExpiresTime {
			r.Status = api_proto.JIT_STATUS_EXPIRED
			changed = true
			logger.Info("JIT grant expired for user %v, roles %v",
				r.Requester, r.Roles)
		}

		// Garbage collect old terminal requests (expired/denied/revoked)
		if r.Status == api_proto.JIT_STATUS_EXPIRED ||
			r.Status == api_proto.JIT_STATUS_DENIED ||
			r.Status == api_proto.JIT_STATUS_REVOKED {
			if r.CreatedTime > 0 && now-r.CreatedTime > maxRequestAge {
				delete(self.requests, id)
				changed = true
			}
		}
	}

	if changed {
		self.saveLocked()
	}
}

func (self *JITManager) load() {
	self.mu.Lock()
	defer self.mu.Unlock()

	file_store_factory := file_store.GetFileStore(self.config_obj)

	reader, err := file_store_factory.ReadFile(jitStorePath)
	if err != nil {
		return
	}
	defer reader.Close()

	data := make([]byte, 0, 4096)
	buf := make([]byte, 4096)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			data = append(data, buf[:n]...)
		}
		if err != nil {
			break
		}
	}

	if len(data) > 0 {
		requests := make(map[string]*api_proto.JITRoleRequest)
		if err := json.Unmarshal(data, &requests); err != nil {
			logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
			logger.Error("JIT Manager: failed to load state file (corrupted?): %v", err)
			return
		}
		self.requests = requests
	}
}

func (self *JITManager) saveLocked() {
	file_store_factory := file_store.GetFileStore(self.config_obj)

	data, err := json.Marshal(self.requests)
	if err != nil {
		return
	}

	writer, err := file_store_factory.WriteFile(jitStorePath)
	if err != nil {
		return
	}
	defer writer.Close()

	writer.Truncate()
	writer.Write(data)
}

func generateRequestID(requester string) string {
	nonce := make([]byte, 8)
	rand.Read(nonce)
	return fmt.Sprintf("jit_%v_%v_%v",
		requester, time.Now().Unix(), hex.EncodeToString(nonce))
}

func (self *JITManager) countPendingForUser(username string) int {
	count := 0
	lower := utils.ToLower(username)
	for _, r := range self.requests {
		if r.Status == api_proto.JIT_STATUS_PENDING &&
			utils.ToLower(r.Requester) == lower {
			count++
		}
	}
	return count
}

func (self *JITManager) RequestRole(
	config_obj *config_proto.Config,
	requester string,
	request *api_proto.JITRequestRoleRequest) (*api_proto.JITRoleRequest, error) {

	if len(request.Roles) == 0 {
		return nil, fmt.Errorf("at least one role must be requested")
	}

	for _, role := range request.Roles {
		if !acls.ValidateRole(role) {
			return nil, fmt.Errorf("invalid role: %v", role)
		}
		if utils.InString(blockedJITRoles, role) {
			return nil, fmt.Errorf(
				"role %q cannot be requested via JIT", role)
		}
	}

	if request.Justification == "" {
		return nil, fmt.Errorf("justification is required")
	}

	if len(request.Justification) > maxJustificationLen {
		return nil, fmt.Errorf("justification too long (max %d characters)",
			maxJustificationLen)
	}

	if request.DurationSec <= 0 {
		return nil, fmt.Errorf("duration must be positive")
	}

	if request.DurationSec > int64(services.MaxJITDurationSec) {
		request.DurationSec = int64(services.MaxJITDurationSec)
	}

	self.mu.Lock()
	defer self.mu.Unlock()

	if self.countPendingForUser(requester) >= maxPendingPerUser {
		return nil, fmt.Errorf(
			"too many pending requests (max %d), please wait for existing requests to be processed",
			maxPendingPerUser)
	}

	request_id := generateRequestID(requester)

	jit_request := &api_proto.JITRoleRequest{
		RequestId:     request_id,
		Requester:     requester,
		Roles:         request.Roles,
		Justification: request.Justification,
		DurationSec:   request.DurationSec,
		OrgId:         request.OrgId,
		Status:        api_proto.JIT_STATUS_PENDING,
		CreatedTime:   time.Now().Unix(),
	}

	self.requests[request_id] = jit_request
	self.saveLocked()

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("JIT role request created by %v for roles %v in org %v",
		requester, request.Roles, request.OrgId)

	return jit_request, nil
}

func (self *JITManager) ApproveOrDeny(
	config_obj *config_proto.Config,
	approver string,
	approval *api_proto.JITApprovalRequest) (*api_proto.JITRoleRequest, error) {

	ok, err := services.CheckAccess(
		config_obj, approver, acls.SERVER_ADMIN)
	if err != nil || !ok {
		return nil, fmt.Errorf("only server_admin can approve/deny JIT requests")
	}

	self.mu.Lock()
	defer self.mu.Unlock()

	request, pres := self.requests[approval.RequestId]
	if !pres {
		return nil, fmt.Errorf("JIT request not found: %v", approval.RequestId)
	}

	if request.Status != api_proto.JIT_STATUS_PENDING {
		return nil, fmt.Errorf("request is not in pending state (current: %v)",
			api_proto.JITRequestStatus_name[int32(request.Status)])
	}

	// Case-insensitive self-approval check
	if utils.ToLower(approver) == utils.ToLower(request.Requester) {
		return nil, fmt.Errorf("cannot approve your own JIT request")
	}

	now := time.Now().Unix()
	request.Approver = approver
	request.ApproverReason = approval.Reason
	request.ApprovedTime = now

	if approval.Approve {
		request.Status = api_proto.JIT_STATUS_APPROVED
		request.ExpiresTime = now + request.DurationSec
	} else {
		request.Status = api_proto.JIT_STATUS_DENIED
	}

	self.saveLocked()

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	action := "denied"
	if approval.Approve {
		action = "approved"
	}
	logger.Info("JIT role request %v %v by %v for user %v",
		approval.RequestId, action, approver, request.Requester)

	return request, nil
}

func (self *JITManager) RevokeGrant(
	config_obj *config_proto.Config,
	principal string,
	request_id string) error {

	ok, err := services.CheckAccess(
		config_obj, principal, acls.SERVER_ADMIN)
	if err != nil || !ok {
		return fmt.Errorf("only server_admin can revoke JIT grants")
	}

	self.mu.Lock()
	defer self.mu.Unlock()

	request, pres := self.requests[request_id]
	if !pres {
		return fmt.Errorf("JIT request not found: %v", request_id)
	}

	if request.Status != api_proto.JIT_STATUS_APPROVED {
		return fmt.Errorf("can only revoke approved grants")
	}

	request.Status = api_proto.JIT_STATUS_REVOKED
	self.saveLocked()

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("JIT grant %v revoked by %v for user %v",
		request_id, principal, request.Requester)

	return nil
}

func (self *JITManager) ListRequests(
	config_obj *config_proto.Config,
	status api_proto.JITRequestStatus,
	username string) (*api_proto.JITRoleRequests, error) {

	self.mu.Lock()
	defer self.mu.Unlock()

	now := time.Now().Unix()
	result := &api_proto.JITRoleRequests{}

	for _, request := range self.requests {
		if request.Status == api_proto.JIT_STATUS_APPROVED &&
			request.ExpiresTime > 0 && now >= request.ExpiresTime {
			request.Status = api_proto.JIT_STATUS_EXPIRED
		}

		if status >= 0 && request.Status != status {
			continue
		}

		if username != "" &&
			utils.ToLower(request.Requester) != utils.ToLower(username) {
			continue
		}

		result.Items = append(result.Items, request)
	}

	return result, nil
}

func (self *JITManager) GetActiveGrants(
	config_obj *config_proto.Config,
	username string) ([]*api_proto.JITRoleRequest, error) {

	username = utils.ToLower(username)

	self.mu.Lock()
	defer self.mu.Unlock()

	now := time.Now().Unix()
	var result []*api_proto.JITRoleRequest

	for _, request := range self.requests {
		if request.Status != api_proto.JIT_STATUS_APPROVED {
			continue
		}

		if utils.ToLower(request.Requester) != username {
			continue
		}

		if request.ExpiresTime > 0 && now >= request.ExpiresTime {
			continue
		}

		result = append(result, request)
	}

	return result, nil
}
