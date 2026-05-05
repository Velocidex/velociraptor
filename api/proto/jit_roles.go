package proto

type JITRequestStatus int32

const (
	JIT_STATUS_PENDING  JITRequestStatus = 0
	JIT_STATUS_APPROVED JITRequestStatus = 1
	JIT_STATUS_DENIED   JITRequestStatus = 2
	JIT_STATUS_EXPIRED  JITRequestStatus = 3
	JIT_STATUS_REVOKED  JITRequestStatus = 4
)

var JITRequestStatus_name = map[int32]string{
	0: "JIT_STATUS_PENDING",
	1: "JIT_STATUS_APPROVED",
	2: "JIT_STATUS_DENIED",
	3: "JIT_STATUS_EXPIRED",
	4: "JIT_STATUS_REVOKED",
}

type JITRoleRequest struct {
	RequestId      string           `json:"request_id,omitempty"`
	Requester      string           `json:"requester,omitempty"`
	Roles          []string         `json:"roles,omitempty"`
	Justification  string           `json:"justification,omitempty"`
	DurationSec    int64            `json:"duration_sec,omitempty"`
	OrgId          string           `json:"org_id,omitempty"`
	Status         JITRequestStatus `json:"status"`
	Approver       string           `json:"approver,omitempty"`
	ApproverReason string           `json:"approver_reason,omitempty"`
	CreatedTime    int64            `json:"created_time,omitempty"`
	ApprovedTime   int64            `json:"approved_time,omitempty"`
	ExpiresTime    int64            `json:"expires_time,omitempty"`
}

type JITRoleRequests struct {
	Items []*JITRoleRequest `json:"items,omitempty"`
}

type JITApprovalRequest struct {
	RequestId string `json:"request_id,omitempty"`
	Approve   bool   `json:"approve"`
	Reason    string `json:"reason,omitempty"`
}

type JITRequestRoleRequest struct {
	Roles         []string `json:"roles,omitempty"`
	Justification string   `json:"justification,omitempty"`
	DurationSec   int64    `json:"duration_sec,omitempty"`
	OrgId         string   `json:"org_id,omitempty"`
}
