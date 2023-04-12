package services

import (
	"context"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

// The Audit Manager is responsible for receiving and dispatching
// audit events. Velociraptor audits user actions and forwards them to
// the audit manager for logging.
type AuditManager interface {
	LogAudit(
		ctx context.Context,
		config_obj *config_proto.Config,
		principal, operation string,
		details *ordereddict.Dict) error
}

func LogAudit(
	ctx context.Context,
	config_obj *config_proto.Config,
	principal, operation string,
	details *ordereddict.Dict) error {
	org_manager, err := GetOrgManager()
	if err != nil {
		return err
	}

	audit_manager, err := org_manager.Services(config_obj.OrgId).AuditManager()
	if err != nil {
		return err
	}

	return audit_manager.LogAudit(ctx, config_obj,
		principal, operation, details)
}
