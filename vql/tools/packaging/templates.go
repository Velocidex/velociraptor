package packaging

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"text/template"

	"github.com/Velocidex/ordereddict"
	"github.com/Velocidex/yaml/v2"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	RPMClientTemplate = map[string]string{
		// Metadata is a JSON encoded struct expanded into the RPM
		// metadata struct.
		// https://github.com/google/rpmpack/blob/2467806670a618497006ff8d8623b0430c7605a9/rpm.go#L56
		"Metadata": `{"name": "{{ .SysvService }}", "version": "{{ .Version }}", "release": "{{ .Release}}", "arch": "{{.Arch}}"}`,

		"ServiceDefinition": `
[Unit]
Description=Velociraptor client
After=syslog.target network.target

[Service]
Type=simple
Restart=always
RestartSec=120
LimitNOFILE=20000
Environment=LANG=en_US.UTF-8
ExecStart={{.VelociraptorBinaryPath}} --config {{.ConfigPath}} client --quiet {{ EscapeArgs .ExtraArgs }}

[Install]
WantedBy=multi-user.target
`,
		"SysvServiceDefinition": `
#!/bin/bash
#
# {{.SysvService}}		Start up the {{.SysvService}} client daemon
#
# chkconfig: 2345 55 25
# description: {{.ServiceDescription}}
#
# processname: {{.SysvService}}
# config: {{.ConfigPath}}
# pidfile: /var/run/{{.SysvService}}.pid

### BEGIN INIT INFO
# Provides: {{.SysvService}}
# Required-Start: $local_fs $network $syslog
# Required-Stop: $local_fs $syslog
# Should-Start: $syslog
# Should-Stop: $network $syslog
# Default-Start: 2 3 4 5
# Default-Stop: 0 1 6
# Short-Description: {{.ServiceDescription}}
### END INIT INFO

# source function library
. /etc/rc.d/init.d/functions

RETVAL=0
prog="{{.SysvService}}"
lockfile=/var/lock/subsys/$prog
VELOCIRAPTOR={{.VelociraptorBinaryPath}}
VELOCIRAPTOR_CONFIG={{.ConfigPath}}
PID_FILE=/var/run/{{.SysvService}}.pid

runlevel=$(set -- $(runlevel); eval "echo \$$#" )

start()
{
	[ -x $VELOCIRAPTOR ] || exit 5
	[ -f $VELOCIRAPTOR_CONFIG ] || exit 6

	echo -n $"Starting $prog: "
	$VELOCIRAPTOR --config "$VELOCIRAPTOR_CONFIG" client &
	RETVAL=$?
	[ $RETVAL -eq 0 ] && /sbin/pidof $prog > $PID_FILE
	echo
	return $RETVAL
}

stop()
{
	echo -n $"Stopping $prog: "
	killproc -p $PID_FILE $VELOCIRAPTOR
	RETVAL=$?
	[ $RETVAL -eq 0 ] && rm -f $lockfile
	echo
}

reload()
{
	echo -n $"Reloading $prog: "
	killproc -p $PID_FILE $VELOCIRAPTOR -HUP
	RETVAL=$?
	echo
}

restart() {
	stop
	start
}

force_reload() {
	restart
}

rh_status() {
	status -p $PID_FILE {{.SysvService}}
}

rh_status_q() {
	rh_status >/dev/null 2>&1
}

case "$1" in
	start)
		rh_status_q && exit 0
		start
		;;
	stop)
		if ! rh_status_q; then
			rm -f $lockfile
			exit 0
		fi
		stop
		;;
	restart)
		restart
		;;
	reload)
		rh_status_q || exit 7
		reload
		;;
	force-reload)
		force_reload
		;;
	condrestart|try-restart)
		rh_status_q || exit 0
		if [ -f $lockfile ] ; then
                 	stop
			# avoid race
			sleep 3
			start
		fi
		;;
	status)
		rh_status
		RETVAL=$?
		if [ $RETVAL -eq 3 -a -f $lockfile ] ; then
			RETVAL=2
		fi
		;;
	*)
		echo $"Usage: $0 {start|stop|restart|reload|force-reload|condrestart|try-restart|status}"
		RETVAL=2
esac
exit $RETVAL
`,

		// RPMs need to support both systemd and sysv based
		// installs. We create the relevant files in this script based
		// on detecting the type of system we are running on.
		"Postin": `
# Lock down permissions on the config file.
chmod -R go-r $(dirname "{{.ConfigPath}}")
chown root:root {{.VelociraptorBinaryPath}}
chmod 755 {{.VelociraptorBinaryPath}}

if [ -f /bin/systemctl ] ; then

cat << SYSTEMDSCRIPT > /etc/systemd/system/{{.SystemdServiceFile}}
{{ ShellEscape ( Expand "ServiceDefinition" ) }}
SYSTEMDSCRIPT

  /bin/systemctl enable {{.SystemdServiceFile}}
  /bin/systemctl start {{.SystemdServiceFile}}

else

cat << SYSVSCRIPT > /etc/rc.d/init.d/{{.SysvService}}
{{ ShellEscape ( Expand "SysvServiceDefinition" ) }}
SYSVSCRIPT

  /bin/chmod +x /etc/rc.d/init.d/{{.SysvService}}

  ## Set it to start at boot
  /sbin/chkconfig --add {{.SysvService}}

  ## Start the service immediately
  service {{.SysvService}} start
fi
`,
		// https://docs.fedoraproject.org/en-US/packaging-guidelines/Scriptlets/#_syntax
		// In preun:
		// $1 == 1 means upgrade
		// $1 == 0 means uninstall
		// Handle both systemd and sysv based systems.
		"Preun": `
if [ -f /bin/systemctl ]; then
    if [ $1 == 1 ] ; then
        /bin/systemctl restart {{.SystemdServiceFile}}
    fi

    if [ $1 == 0 ] ; then
        /bin/systemctl disable {{.SystemdServiceFile}}
        /bin/systemctl stop {{.SystemdServiceFile}}
    fi
else
    if [ $1 == 1 ] ; then
        /sbin/service {{.SysvService}} start  > /dev/null 2>&1 || :
    fi

    if [ $1 == 0 ] ; then
        /sbin/service {{.SysvService}} stop > /dev/null 2>&1 || :
        /sbin/chkconfig --del {{.SysvService}}
    fi
fi
`,

		// In postun:
		// $1 == 1 means upgrade
		// $1 == 0 means uninstall
		//
		// Remove service file only on uninstall
		"Postun": `
if [ -f /bin/systemctl ] ; then
    if [ $1 = 0 ] ; then
       rm /etc/systemd/system/{{.SystemdServiceFile}}
    fi
else
    if [ $1 = 0 ] ; then
       rm /etc/rc.d/init.d/{{.SysvService}}
    fi
fi
`,
	}

	RPMServerTemplates = map[string]string{
		"Metadata": `{"name": "{{ .SysvService }}", "version": "{{ .Version }}", "release": "{{ .Release}}", "arch": "{{.Arch}}"}`,

		"ServiceDefinition": `
[Unit]
Description={{.ServiceDescription}}
After=syslog.target network.target

[Service]
Type=simple
Restart=always
RestartSec=120
LimitNOFILE=20000
Environment=LANG=en_US.UTF-8
ExecStart={{.VelociraptorBinaryPath}} --config {{.ConfigPath}} frontend {{ EscapeArgs .ExtraArgs }}
{{- if eq .Variant "minion" -}}
   --minion --node {{ .NodeName }}
{{- end }}
User={{.ServerUser}}
Group={{.ServerUser}}
CapabilityBoundingSet=CAP_SYS_RESOURCE CAP_NET_BIND_SERVICE
AmbientCapabilities=CAP_SYS_RESOURCE CAP_NET_BIND_SERVICE

[Install]
WantedBy=multi-user.target
`,
		"Preinst": `
if ! [ -f /bin/systemctl ] ; then
   getent group {{.ServerUser}} >/dev/null || groupadd -g 115 -r {{.ServerUser}} || :
   getent passwd {{.ServerUser}} >/dev/null || \
   useradd -c "Privilege-separated {{.ServerUser}}" -u 115 -g {{.ServerUser}}  -s /sbin/nologin \
     -s /sbin/nologin -r -d /var/empty/{{.ServerUser}} {{.ServerUser}} 2> /dev/null || :
fi
`,

		"PostInst": `
getent group {{.ServerUser}} >/dev/null 2>&1 || groupadd \
        -r \
        {{.ServerUser}}
getent passwd {{.ServerUser}} >/dev/null 2>&1 || useradd \
        -r -l \
        -g {{.ServerUser}} \
        -d /proc \
        -s /sbin/nologin \
        -c "{{.ServiceDescription}}" \
        {{.ServerUser}}
:;
{{ Expand "CommonPostinst" }}
`,
		// Prepare the system to be a server - lock down permissions
		// on the datastore.
		"CommonPostinst": `
# Make the filestore path accessible to the user.
mkdir -p '{{.FileStorePath}}'/config

# Only chown two levels of the filestore directory in case
# this is an upgrade and there are many files already there.
# otherwise chown -R takes too long.
chown {{.ServerUser}}:{{.ServerUser}} '{{.FileStorePath}}' '{{.FileStorePath}}'/*
chown {{.ServerUser}}:{{.ServerUser}} -R $(dirname "{{.ConfigPath}}")

# Lock down permissions on the config file.
chmod -R go-r $(dirname "{{.ConfigPath}}")
chown root:root {{.VelociraptorBinaryPath}}
chmod 755 {{.VelociraptorBinaryPath}}

# Allow the server to bind to low ports and increase its fd limit.
setcap CAP_SYS_RESOURCE,CAP_NET_BIND_SERVICE=+eip {{.VelociraptorBinaryPath}}
/bin/systemctl enable {{.SystemdServiceFile}}
/bin/systemctl start {{.SystemdServiceFile}}
`,
		"Preun": `
/bin/systemctl disable {{.SystemdServiceFile}}
/bin/systemctl stop {{.SystemdServiceFile}}
`,
	}

	DebClientTemplates = map[string]string{
		"ServiceDefinition": RPMClientTemplate["ServiceDefinition"],
		"PostInst": `
# Lock down permissions on the config file.
chmod -R go-r $(dirname "{{.ConfigPath}}")
chown root:root {{.VelociraptorBinaryPath}}
chmod 755 {{.VelociraptorBinaryPath}}

/bin/systemctl enable {{.SystemdServiceFile}}
/bin/systemctl start {{.SystemdServiceFile}}
`,
		"Prerm": `
/bin/systemctl disable {{.SystemdServiceFile}}
/bin/systemctl stop {{.SystemdServiceFile}}
`,
	}

	DebServerTemplates = map[string]string{
		"ServiceDefinition": RPMServerTemplates["ServiceDefinition"],
		"Prerm":             DebClientTemplates["Prerm"],

		// Add privilege separated user accounts and lock down
		// permissions.
		"PostInst": `
if ! getent group {{.ServerUser}} >/dev/null; then
   addgroup --system {{.ServerUser}}
fi

if ! getent passwd {{.ServerUser}} >/dev/null; then
   adduser --system --home /etc/{{.ServerUser}} --no-create-home \
     --ingroup {{.ServerUser}} {{.ServerUser}} --shell /bin/false \
     --gecos "{{.ServiceDescription}}"
fi

{{ Expand "CommonPostinst" }}
`,
		"CommonPostinst": RPMServerTemplates["CommonPostinst"],
	}
)

type TemplateExpansion struct {
	// Used by Deb Packages
	Name, Maintainer, MaintainerEmail, Homepage, Depends string

	Version string
	Release string
	Arch    string

	// Where the config file will be installed to
	ConfigPath string

	// Where the velociraptor binary is installed to
	VelociraptorBinaryPath string

	// Additional commands to append to the service command line
	ExtraArgs string

	// The privilege separated username for the server.
	ServerUser string

	// Where to put the service file on systemd
	SystemdServiceFile string

	// Name of the sysv service to create
	SysvService string

	// The description of the service in the service file.
	ServiceDescription string

	// Path to the filestore
	FileStorePath string

	ConfigYaml   string
	ExeBytes     string
	NodeName     string
	Variant      string // master/minion or empty
	MinionNumber int
}

type FileSpec struct {
	RawData  []byte
	Mode     uint
	Owner    string
	Template string
}

type PackageSpec struct {
	// Set to true for server packages
	Server bool

	// The package will have this filename.
	OutputFilenameTemplate string

	// A list of files to stored in the package.
	Files *ordereddict.Dict // map[string]FileSpec

	// A set of template definitions to expand in the Files above.
	Templates map[string]string

	// A common set of expansion variables.
	Expansion TemplateExpansion
}

func (self *PackageSpec) Copy() *PackageSpec {
	return &PackageSpec{
		OutputFilenameTemplate: self.OutputFilenameTemplate,
		Files:                  self.Files,
		Templates:              self.Templates,
		Expansion:              self.Expansion,
	}
}

func (self *PackageSpec) SetRuntimeParameters(
	config_obj *config_proto.Config,
	arch, release, variant string,
	minion_number int,
	exe_bytes []byte) *PackageSpec {
	version := strings.ReplaceAll(config_obj.Version.Version, "-", ".")

	config_yaml, _ := yaml.Marshal(config_obj)
	self.Expansion.ConfigYaml = string(config_yaml)
	self.Expansion.ExeBytes = string(exe_bytes)
	self.Expansion.Version = version
	self.Expansion.Arch = arch
	self.Expansion.Release = release
	self.Expansion.Variant = variant
	self.Expansion.MinionNumber = minion_number

	if config_obj.Datastore != nil {
		self.Expansion.FileStorePath = config_obj.Datastore.Location
	}

	if config_obj.Frontend != nil {
		if variant == "minion" {
			idx := minion_number % len(config_obj.ExtraFrontends)
			self.Expansion.NodeName = services.GetNodeName(config_obj.ExtraFrontends[idx])

		} else {
			self.Expansion.NodeName = services.GetNodeName(config_obj.Frontend)
		}
	}

	return self
}

func (self *PackageSpec) OutputFilename() string {
	res, _ := ExpandTemplateString(self.OutputFilenameTemplate,
		self.Expansion, self.Templates)
	return res
}

func NewClientRPMSpec() *PackageSpec {
	return &PackageSpec{
		OutputFilenameTemplate: "velociraptor_client_{{ .Version }}_{{ .Arch }}.rpm",
		Files: ordereddict.NewDict().
			Set("{{.ConfigPath}}", FileSpec{
				Template: `{{ .ConfigYaml }}`,
				Mode:     0600,
				Owner:    "root",
			}).
			Set("{{.VelociraptorBinaryPath}}", FileSpec{
				Template: `{{ .ExeBytes }}`,
				Mode:     0755,
				Owner:    "root",
			}).
			Set("Postin", FileSpec{
				Template: `{{ Expand "Postin" }}`,
			}).
			Set("Preun", FileSpec{
				Template: `{{ Expand "Preun" }}`,
			}).
			Set("Postun", FileSpec{
				Template: `{{ Expand "Postun" }}`,
			}),
		Templates: RPMClientTemplate,
		Expansion: TemplateExpansion{
			Name:            "velociraptor-client",
			Release:         "A",
			Maintainer:      "Velocidex Enterprises",
			MaintainerEmail: "support@velocidex.com",
			Homepage:        "https://www.velocidex.com",
			Depends:         "libcap2-bin, systemd",

			ConfigPath:             "/etc/velociraptor/client.config.yaml",
			VelociraptorBinaryPath: "/usr/local/bin/velociraptor_client",
			ServerUser:             "velociraptor",
			SystemdServiceFile:     "velociraptor_client.service",
			SysvService:            "velociraptor_client",
			ServiceDescription:     "Velociraptor is an endpoint monitoring tool",
		},
	}
}

func NewClientDebSpec() *PackageSpec {
	return &PackageSpec{
		OutputFilenameTemplate: "velociraptor_client_{{ .Version }}_{{ .Arch }}.deb",
		Files: ordereddict.NewDict().
			Set("{{.ConfigPath}}", FileSpec{
				Template: `{{ .ConfigYaml }}`,
				Mode:     0600,
				Owner:    "root",
			}).
			Set("/etc/systemd/system/{{.SystemdServiceFile}}", FileSpec{
				Template: `{{ Expand "ServiceDefinition" }}`,
			}).
			Set("{{.VelociraptorBinaryPath}}", FileSpec{
				Template: `{{ .ExeBytes }}`,
				Mode:     0755,
				Owner:    "root",
			}).
			Set("Postin", FileSpec{
				Template: `{{ Expand "PostInst" }}`,
			}).
			Set("Prerm", FileSpec{
				Template: `{{ Expand "Prerm" }}`,
			}),
		Templates: DebClientTemplates,
		Expansion: TemplateExpansion{
			Name:            "velociraptor-client",
			Maintainer:      "Velocidex Enterprises",
			MaintainerEmail: "support@velocidex.com",
			Homepage:        "https://www.velocidex.com",

			ConfigPath:             "/etc/velociraptor/client.config.yaml",
			VelociraptorBinaryPath: "/usr/local/bin/velociraptor_client",
			ServerUser:             "velociraptor",
			SystemdServiceFile:     "velociraptor_client.service",
			SysvService:            "velociraptor_client",
			ServiceDescription:     "Velociraptor client package.",
		},
	}
}

func NewServerRPMSpec() *PackageSpec {
	return &PackageSpec{
		Server: true,
		OutputFilenameTemplate: `{{- if eq .Variant "master" -}}
velociraptor-server-master-{{ .Version }}.{{ .Arch }}-{{ .NodeName}}.rpm
{{- else if eq .Variant "minion" -}}
velociraptor-server-minion-{{ .Version }}.{{ .Arch }}-{{ .NodeName}}.rpm
{{- else -}}
velociraptor-server-{{ .Version }}.{{ .Arch }}.rpm
{{- end -}}
`,
		Files: ordereddict.NewDict().
			Set("{{.ConfigPath}}", FileSpec{
				Template: `{{ .ConfigYaml }}`,
				Mode:     0600,
				Owner:    "root",
			}).
			Set("{{.VelociraptorBinaryPath}}", FileSpec{
				Template: `{{ .ExeBytes }}`,
				Mode:     0755,
				Owner:    "root",
			}).
			Set("/etc/systemd/system/{{.SystemdServiceFile}}", FileSpec{
				Template: `{{ Expand "ServiceDefinition" }}`,
				Mode:     0644,
				Owner:    "root",
			}).
			Set("Postin", FileSpec{
				Template: `{{ Expand "PostInst" }}`,
			}).
			Set("Preun", FileSpec{
				Template: `{{ Expand "Preun" }}`,
			}),
		Templates: RPMServerTemplates,
		Expansion: TemplateExpansion{
			Name:            "velociraptor-server",
			Maintainer:      "Velocidex Enterprises",
			MaintainerEmail: "support@velocidex.com",
			Homepage:        "https://www.velocidex.com",
			Depends:         "libcap2-bin, systemd",

			ConfigPath:             "/etc/velociraptor/server.config.yaml",
			VelociraptorBinaryPath: "/usr/local/bin/velociraptor",
			ServerUser:             "velociraptor",
			SystemdServiceFile:     "velociraptor_server.service",
			SysvService:            "velociraptor",
			ServiceDescription:     "Velociraptor server",
		},
	}
}

func NewServerDebSpec() *PackageSpec {
	return &PackageSpec{
		Server: true,
		OutputFilenameTemplate: `{{- if eq .Variant "master" -}}
velociraptor-server-master-{{ .Version }}.{{ .Arch }}-{{ .NodeName}}.deb
{{- else if eq .Variant "minion" -}}
velociraptor-server-minion-{{ .Version }}.{{ .Arch }}-{{ .NodeName}}.deb
{{- else -}}
velociraptor-server-{{ .Version }}.{{ .Arch }}.deb
{{- end -}}
`,
		Files: ordereddict.NewDict().
			Set("{{.ConfigPath}}", FileSpec{
				Template: `{{ .ConfigYaml }}`,
				Mode:     0600,
				Owner:    "root",
			}).
			Set("{{.VelociraptorBinaryPath}}", FileSpec{
				Template: `{{ .ExeBytes }}`,
				Mode:     0755,
				Owner:    "root",
			}).
			Set("/etc/systemd/system/{{.SystemdServiceFile}}", FileSpec{
				Template: `{{ Expand "ServiceDefinition" }}`,
				Mode:     0644,
				Owner:    "root",
			}).
			Set("Postin", FileSpec{
				Template: `{{ Expand "PostInst" }}`,
			}).
			Set("Prerm", FileSpec{
				Template: `{{ Expand "Prerm" }}`,
			}),
		Templates: DebServerTemplates,
		Expansion: TemplateExpansion{
			Name:            "velociraptor-server",
			Maintainer:      "Velocidex Enterprises",
			MaintainerEmail: "support@velocidex.com",
			Homepage:        "https://www.velocidex.com",
			Depends:         "libcap2-bin, systemd",

			ConfigPath:             "/etc/velociraptor/server.config.yaml",
			VelociraptorBinaryPath: "/usr/local/bin/velociraptor",
			ServerUser:             "velociraptor",
			SystemdServiceFile:     "velociraptor_server.service",
			SysvService:            "velociraptor",
			ServiceDescription:     "Velociraptor server",
		},
	}
}

func ExpandTemplate(
	name string,
	expansion TemplateExpansion,
	template_defs map[string]string) (string, error) {
	data, pres := template_defs[name]
	if !pres {
		return "", utils.Wrap(utils.NotFoundError, "No template known "+name)
	}

	return ExpandTemplateString(data, expansion, template_defs)
}

func ExpandTemplateString(
	data string,
	expansion TemplateExpansion,
	template_defs map[string]string) (string, error) {
	tmpl, err := template.New("").Funcs(
		template.FuncMap{
			"Expand": func(templates ...string) interface{} {
				if len(templates) != 1 {
					return "<Error: Expand must be given the template name>"
				}

				value, err := ExpandTemplate(templates[0], expansion, template_defs)
				if err != nil {
					return fmt.Sprintf("<Error: %v>", err)
				}
				return value
			},

			"ShellEscape": func(str ...string) interface{} {
				if len(str) != 1 {
					return "<Error: ShellEscape must be given a single string>"
				}

				in := strings.ReplaceAll(str[0], "\\", "\\\\")
				return strings.ReplaceAll(in, "$", "\\$")
			},

			"EscapeArgs": func(str ...string) interface{} {
				return EscapeArgv(str)
			},
		}).Parse(string(data))
	if err != nil {
		return "", err
	}

	buff := &bytes.Buffer{}
	err = tmpl.Execute(buff, expansion)
	return buff.String(), err
}

var shellShouldEscape = regexp.MustCompile(`[^\w@%+=:,./-]`)

func EscapeArgv(argv []string) string {
	var res []string

	for _, arg := range argv {
		if len(arg) == 0 {
			continue
		}

		if arg[0] == '\'' && arg[len(arg)-1] == '\'' {
			res = append(res, arg)
			continue
		}

		if shellShouldEscape.MatchString(arg) {
			res = append(res, "'"+strings.ReplaceAll(arg, "'", "'\"'\"'")+"'")
			continue
		}

		res = append(res, arg)
	}

	return strings.Join(res, " ")
}
