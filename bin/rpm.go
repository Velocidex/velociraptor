package main

import (
	"debug/elf"
	"fmt"
	"os"
	"strings"

	"github.com/Velocidex/yaml/v2"
	"github.com/google/rpmpack"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	logging "www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/startup"
)

var (
	rpm_command = app.Command(
		"rpm", "Create an rpm package")

	rpm_command_release = rpm_command.Flag(
		"release", "Rpm package release version").Default("A").String()

	client_rpm_command = rpm_command.Command(
		"client", "Create a client package from a server config file.")

	server_rpm_command = rpm_command.Command(
		"server", "Create a server package from a server config file.")

	server_rpm_command_output = server_rpm_command.Flag(
		"output", "Filename to output").String()

	server_rpm_command_binary = server_rpm_command.Flag(
		"binary", "The binary to package").String()

	client_rpm_command_use_sysv = client_rpm_command.Flag(
		"use_sysv", "Use SysV style services (CentOS 6)").Bool()

	client_rpm_command_output = client_rpm_command.Flag(
		"output", "Filename to output").String()

	client_rpm_command_binary = client_rpm_command.Flag(
		"binary", "The binary to package").String()

	server_rpm_post_install_template = `
getent group velociraptor >/dev/null 2>&1 || groupadd \
        -r \
        velociraptor
getent passwd velociraptor >/dev/null 2>&1 || useradd \
        -r -l \
        -g velociraptor \
        -d /proc \
        -s /sbin/nologin \
        -c "Velociraptor Server" \
        velociraptor
:;

# Make the filestore path accessible to the user.
mkdir -p '%s'/config

# Only chown two levels of the filestore directory in case
# this is an upgrade and there are many files already there.
# otherwise chown -R takes too long.
chown velociraptor:velociraptor '%s' '%s'/*
chown velociraptor:velociraptor -R /etc/velociraptor/

# Lock down permissions on the config file.
chmod -R go-r /etc/velociraptor/
chmod o+x /usr/local/bin/velociraptor /usr/local/bin/velociraptor

# Allow the server to bind to low ports and increase its fd limit.
setcap CAP_SYS_RESOURCE,CAP_NET_BIND_SERVICE=+eip /usr/local/bin/velociraptor
/bin/systemctl enable velociraptor_server.service
/bin/systemctl start velociraptor_server.service
`

	rpm_sysv_client_service_definition = `
#!/bin/bash
#
# velociraptor		Start up the Velociraptor client daemon
#
# chkconfig: 2345 55 25
# description: Velociraptor is an endpoint monitoring tool
#
# processname: velociraptor
# config: /etc/velociraptor/client.config.yaml
# pidfile: /var/run/velociraptor.pid

### BEGIN INIT INFO
# Provides: velociraptor
# Required-Start: $local_fs $network $syslog
# Required-Stop: $local_fs $syslog
# Should-Start: $syslog
# Should-Stop: $network $syslog
# Default-Start: 2 3 4 5
# Default-Stop: 0 1 6
# Short-Description: Start up the Velociraptor client daemon
### END INIT INFO

# source function library
. /etc/rc.d/init.d/functions

RETVAL=0
prog="velociraptor"
lockfile=/var/lock/subsys/$prog
VELOCIRAPTOR=/usr/local/bin/velociraptor
VELOCIRAPTOR_CONFIG=/etc/velociraptor/client.config.yaml
PID_FILE=/var/run/velociraptor.pid

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
	status -p $PID_FILE velociraptor
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
`

	// rpmArchMap maps ELF machine strings to RPM architectures
	// See https://github.com/torvalds/linux/blob/master/include/uapi/linux/elf-em.h
	//     https://fedoraproject.org/wiki/Architectures
	rpmArchMap = map[string]string{
		"EM_X86_64":  "x86_64",
		"EM_386":     "i386",
		"EM_AARCH64": "aarch64",
		"EM_RISCV":   "riscv64",
		"EM_ARM":     "armhfp",
		"EM_PPC64":   "ppc64le",
	}
)

// Systemd based start up scripts (Centos 7, 8)
func doClientRPM() error {
	// Disable logging when creating a package - we may not create the
	// package on the same system where the logs should go.
	logging.DisableLogging()

	config_obj, err := makeDefaultConfigLoader().
		WithRequiredClient().LoadAndValidate()
	if err != nil {
		return fmt.Errorf("Unable to load config file: %w", err)
	}

	ctx, cancel := install_sig_handler()
	defer cancel()

	sm, err := startup.StartToolServices(ctx, config_obj)
	defer sm.Close()

	if err != nil {
		return err
	}

	config_file_yaml, err := yaml.Marshal(getClientConfig(config_obj))
	if err != nil {
		return err
	}

	input := *client_rpm_command_binary

	if input == "" {
		input, err = os.Executable()
		if err != nil {
			return fmt.Errorf("Unable to find executable: %w", err)
		}
	}

	e, err := elf.Open(input)
	if err != nil {
		return fmt.Errorf("Unable to parse ELF executable: %w", err)
	}

	arch, ok := rpmArchMap[e.Machine.String()]
	if !ok {
		return fmt.Errorf("unknown binary architecture: %q", e.Machine.String())
	}

	binary_content, err := os.ReadFile(input)
	if err != nil {
		return fmt.Errorf("Unable to read executable: %w", err)
	}

	version := strings.ReplaceAll(constants.VERSION, "-", ".")

	output_path := fmt.Sprintf("velociraptor_client_%s_%s.rpm", version, arch)
	if *client_rpm_command_output != "" {
		output_path = *client_rpm_command_output
	}

	fmt.Printf("Creating client package at %s\n", output_path)

	r, err := rpmpack.NewRPM(rpmpack.RPMMetaData{
		Name:    "velociraptor-client",
		Version: version,
		Release: *rpm_command_release,
		Arch:    arch,
	})
	if err != nil {
		return fmt.Errorf("Unable to create RPM: %w", err)
	}

	r.AddFile(
		rpmpack.RPMFile{
			Name:  "/etc/velociraptor/client.config.yaml",
			Mode:  0600,
			Body:  config_file_yaml,
			Owner: "root",
			Group: "root",
		})

	r.AddFile(
		rpmpack.RPMFile{
			Name:  "/usr/local/bin/velociraptor_client",
			Body:  binary_content,
			Mode:  0755,
			Owner: "root",
			Group: "root",
		})

	config_path := "/etc/velociraptor/client.config.yaml"
	velociraptor_bin := "/usr/local/bin/velociraptor_client"

	r.AddFile(
		rpmpack.RPMFile{
			Name: "/etc/systemd/system/velociraptor_client.service",
			Body: []byte(fmt.Sprintf(
				client_service_definition, velociraptor_bin, config_path)),
			Mode:  0644,
			Owner: "root",
			Group: "root",
		})

	r.AddPostin(`/bin/systemctl enable velociraptor_client.service
/bin/systemctl start velociraptor_client.service
`)

	// check for upgrade vs uninstall
	r.AddPreun(`
if [ $1 == 1 ] ; then
    /bin/systemctl restart velociraptor_client.service
fi

if [ $1 == 0 ] ; then
    /bin/systemctl disable velociraptor_client.service
    /bin/systemctl stop velociraptor_client.service
fi
`)

	fd, err := os.OpenFile(output_path,
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("Unable to create output file: %w", err)
	}
	defer fd.Close()

	return r.Write(fd)
}

// Systemd based start up scripts (CentOS 7+)
func doServerRPM() error {
	// Disable logging when creating a package - we may not create the
	// package on the same system where the logs should go.
	logging.DisableLogging()

	config_obj, err := makeDefaultConfigLoader().
		WithRequiredFrontend().LoadAndValidate()
	if err != nil {
		return fmt.Errorf("Unable to load config file: %w", err)
	}

	if len(config_obj.ExtraFrontends) == 0 {
		return doSingleServerRPM(config_obj, "", nil)
	}

	// Build the master node
	node_name := services.GetNodeName(config_obj.Frontend)
	err = doSingleServerRPM(config_obj, "_master_"+node_name, nil)
	if err != nil {
		return err
	}

	for _, fe := range config_obj.ExtraFrontends {
		node_name := services.GetNodeName(fe)
		err = doSingleServerRPM(config_obj, "_minion_"+node_name,
			[]string{"--minion", "--node", node_name})
		if err != nil {
			return err
		}
	}

	return nil
}

func doSingleServerRPM(
	config_obj *config_proto.Config,
	variant string, extra_args []string) error {
	// Linux packages always use the "velociraptor" user.
	config_obj.Frontend.RunAsUser = "velociraptor"
	config_obj.ServerType = "linux"

	var err error

	input := *server_rpm_command_binary
	if input == "" {
		input, err = os.Executable()
		if err != nil {
			return fmt.Errorf("Unable to find executable: %w", err)
		}
	}

	e, err := elf.Open(input)
	if err != nil {
		return fmt.Errorf("Unable to parse ELF executable: %w", err)
	}

	arch, ok := rpmArchMap[e.Machine.String()]
	if !ok {
		return fmt.Errorf("unknown binary architecture: %q", e.Machine.String())
	}

	binary_content, err := os.ReadFile(input)
	if err != nil {
		return fmt.Errorf("Unable to read executable: %w", err)
	}

	config_file_yaml, err := yaml.Marshal(config_obj)
	if err != nil {
		return err
	}

	version := strings.ReplaceAll(constants.VERSION, "-", ".")

	kind := "server"
	if variant != "" {
		kind = kind + "-" + variant
	}

	output_path := fmt.Sprintf("velociraptor-%s-%s.%s.rpm", kind, version, arch)
	if *server_rpm_command_output != "" {
		output_path = *server_rpm_command_output
		if variant != "" {
			output_path = strings.TrimSuffix(output_path, ".rpm") + variant + ".rpm"
		}
	}

	fmt.Printf("Creating %s package at %s\n", variant, output_path)

	r, err := rpmpack.NewRPM(rpmpack.RPMMetaData{
		Name:    "velociraptor-server",
		Version: version,
		Release: *rpm_command_release,
		Arch:    arch,
	})
	if err != nil {
		return fmt.Errorf("Unable to create RPM: %w", err)
	}

	r.AddFile(
		rpmpack.RPMFile{
			Name:  "/etc/velociraptor/server.config.yaml",
			Mode:  0600,
			Body:  config_file_yaml,
			Owner: "root",
			Group: "root",
		})

	r.AddFile(
		rpmpack.RPMFile{
			Name:  "/usr/local/bin/velociraptor",
			Body:  binary_content,
			Mode:  0755,
			Owner: "root",
			Group: "root",
		})

	config_path := "/etc/velociraptor/server.config.yaml"
	velociraptor_bin := "/usr/local/bin/velociraptor"

	r.AddFile(
		rpmpack.RPMFile{
			Name: "/etc/systemd/system/velociraptor_server.service",
			Body: []byte(fmt.Sprintf(
				server_service_definition, velociraptor_bin, config_path,
				strings.Join(extra_args, " "))),
			Mode:  0644,
			Owner: "root",
			Group: "root",
		})

	filestore_path := config_obj.Datastore.Location
	r.AddPostin(fmt.Sprintf(server_rpm_post_install_template,
		filestore_path, filestore_path, filestore_path))

	r.AddPreun(`
/bin/systemctl disable velociraptor_server.service
/bin/systemctl stop velociraptor_server.service
`)

	fd, err := os.OpenFile(output_path,
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("Unable to create output file: %w", err)
	}
	defer fd.Close()

	return r.Write(fd)
}

// Simple startup scripts for SysV-style init systems (Centos 6)
func doClientSysVRPM() error {
	// Disable logging when creating a package - we may not create the
	// package on the same system where the logs should go.
	logging.DisableLogging()

	config_obj, err := makeDefaultConfigLoader().
		WithRequiredClient().LoadAndValidate()
	if err != nil {
		return fmt.Errorf("Unable to load config file: %w", err)
	}

	ctx, cancel := install_sig_handler()
	defer cancel()

	sm, err := startup.StartToolServices(ctx, config_obj)
	defer sm.Close()

	if err != nil {
		return err
	}

	config_file_yaml, err := yaml.Marshal(getClientConfig(config_obj))
	if err != nil {
		return err
	}

	input := *client_rpm_command_binary

	if input == "" {
		input, err = os.Executable()
		if err != nil {
			return fmt.Errorf("Unable to find executable: %w", err)
		}
	}

	e, err := elf.Open(input)
	if err != nil {
		return fmt.Errorf("Unable to parse ELF executable: %w", err)
	}

	arch, ok := rpmArchMap[e.Machine.String()]
	if !ok {
		return fmt.Errorf("unknown binary architecture: %q", e.Machine.String())
	}

	binary_content, err := os.ReadFile(input)
	if err != nil {
		return fmt.Errorf("Unable to read executable: %w", err)
	}

	version := strings.ReplaceAll(constants.VERSION, "-", ".")

	output_path := fmt.Sprintf("velociraptor_client_%s_%s.rpm", version, arch)
	if *client_rpm_command_output != "" {
		output_path = *client_rpm_command_output
	}

	fmt.Printf("Creating SysV-init client package at %s\n", output_path)

	r, err := rpmpack.NewRPM(rpmpack.RPMMetaData{
		Name:    "velociraptor-client",
		Version: version,
		Release: *rpm_command_release,
		Arch:    arch,
	})
	if err != nil {
		return fmt.Errorf("Unable to create RPM: %w", err)
	}
	r.AddFile(
		rpmpack.RPMFile{
			Name:  "/etc/velociraptor/client.config.yaml",
			Mode:  0600,
			Body:  config_file_yaml,
			Owner: "root",
			Group: "root",
		})
	r.AddFile(
		rpmpack.RPMFile{
			Name:  "/usr/local/bin/velociraptor",
			Body:  binary_content,
			Mode:  0755,
			Owner: "root",
			Group: "root",
		})
	r.AddFile(
		rpmpack.RPMFile{
			Name:  "/etc/rc.d/init.d/velociraptor",
			Body:  []byte(rpm_sysv_client_service_definition),
			Mode:  0755,
			Owner: "root",
			Group: "root",
		})

	r.AddPrein(`
getent group velociraptor >/dev/null || groupadd -g 115 -r velociraptor || :
getent passwd velociraptor >/dev/null || \
useradd -c "Privilege-separated Velociraptor" -u 115 -g velociraptor  -s /sbin/nologin \
  -s /sbin/nologin -r -d /var/empty/velociraptor velociraptor 2> /dev/null || :
`)

	r.AddPostin("/sbin/chkconfig --add velociraptor")
	r.AddPreun(`
if [ "$1" = 0 ]
then
        /sbin/service velociraptor stop > /dev/null 2>&1 || :
        /sbin/chkconfig --del velociraptor
fi
`)
	r.AddPostun(`
/sbin/service velociraptor start  > /dev/null 2>&1 || :
`)

	fd, err := os.OpenFile(output_path,
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("Unable to create output file: %w", err)
	}
	defer fd.Close()

	return r.Write(fd)
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case client_rpm_command.FullCommand():
			if *client_rpm_command_use_sysv {
				FatalIfError(client_rpm_command, doClientSysVRPM)
			} else {
				FatalIfError(client_rpm_command, doClientRPM)
			}

		case server_rpm_command.FullCommand():
			FatalIfError(server_rpm_command, doServerRPM)

		default:
			return false
		}
		return true
	})
}
