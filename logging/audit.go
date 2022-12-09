package logging

import (
	"github.com/sirupsen/logrus"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

// A wrapper around audit logging. Audit events need to have more
// structure than the other events, so they can be easily
// searched. This wrapper ensures the minimal amount of information is
// included in the event.
func LogAudit(
	config_obj *config_proto.Config,
	principal, operation string,
	details logrus.Fields) {

	details["principal"] = principal
	logger := GetLogger(config_obj, &Audit)
	logger.WithFields(details).Info(operation)
}
