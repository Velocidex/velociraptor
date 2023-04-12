package logging

import (
	"log/syslog"
	"strings"

	lSyslog "github.com/sirupsen/logrus/hooks/syslog"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

func maybeAddRemoteSyslog(
	config_obj *config_proto.Config, manager *LogManager) error {

	if config_obj.Logging == nil ||
		config_obj.Logging.RemoteSyslogServer == "" {
		return nil
	}

	protocol := config_obj.Logging.RemoteSyslogProtocol
	if protocol == "" {
		protocol = "udp"
	}

	server := config_obj.Logging.RemoteSyslogServer
	if !strings.Contains(server, ":") {
		server += ":514"
	}

	components := make(map[*string]bool)

	for _, c := range config_obj.Logging.RemoteSyslogComponents {
		switch c {
		case GenericComponent:
			components[&GenericComponent] = true
		case FrontendComponent:
			components[&FrontendComponent] = true
		case ClientComponent:
			components[&ClientComponent] = true
		case GUIComponent:
			components[&GUIComponent] = true
		case ToolComponent:
			components[&ToolComponent] = true
		case APICmponent:
			components[&APICmponent] = true
		case Audit:
			components[&Audit] = true
		}
	}

	// If no components are specified only forward the audit logs.
	if len(components) == 0 {
		components[&Audit] = true
	}

	Prelog("<green>Will connect to syslog server %v over %v</>",
		server, protocol)

	hook, err := lSyslog.NewSyslogHook(
		protocol, server, syslog.LOG_INFO, "")
	if err != nil {
		Prelog("While connecting to Syslog %v: %v", server, err)
		return err
	}

	for k, v := range manager.contexts {
		_, pres := components[k]
		if pres {
			v.Logger.Hooks.Add(hook)
		}
	}

	return nil
}
