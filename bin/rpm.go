package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"

	"github.com/Velocidex/yaml/v2"
	"github.com/google/rpmpack"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/startup"
)

var (
	rpm_command = app.Command(
		"rpm", "Create an rpm package")

	client_rpm_command = rpm_command.Command(
		"client", "Create a client package from a server config file.")

	server_rpm_command = rpm_command.Command(
		"server", "Create a server package from a server config file.")

	server_rpm_command_output = server_rpm_command.Flag(
		"output", "Filename to output").Default(
		fmt.Sprintf("velociraptor_%s_server.rpm", constants.VERSION)).
		String()

	server_rpm_command_binary = server_rpm_command.Flag(
		"binary", "The binary to package").String()

	client_rpm_command_use_sysv = client_rpm_command.Flag(
		"use_sysv", "Use sys V style services (Centos 6)").Bool()

	client_rpm_command_output = client_rpm_command.Flag(
		"output", "Filename to output").Default(
		fmt.Sprintf("velociraptor_%s_client.rpm", constants.VERSION)).
		String()

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
)

// Systemd based start up scripts (Centos 7, 8)
func doClientRPM() error {
	// Disable logging when creating a deb - we may not create the
	// deb on the same system where the logs should go.
	_ = config.ValidateClientConfig(&config_proto.Config{})

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
			return fmt.Errorf("Unable to open executable: %w", err)
		}
	}

	fd, err := os.Open(input)
	if err != nil {
		return fmt.Errorf("Unable to open executable: %w", err)
	}
	defer fd.Close()

	header := make([]byte, 4)
	_, err = fd.Read(header)
	if err != nil {
		return fmt.Errorf("Unable to open executable: %w", err)
	}

	if binary.LittleEndian.Uint32(header) != 0x464c457f {
		return fmt.Errorf("Binary does not appear to be an " +
			"ELF binary. Please specify the linux binary " +
			"using the --binary flag.")
	}

	_, err = fd.Seek(0, io.SeekStart)
	if err != nil {
		return fmt.Errorf("Unable to read executable: %w", err)
	}

	binary_text, err := ioutil.ReadAll(fd)
	if err != nil {
		return fmt.Errorf("Unable to read executable: %w", err)
	}
	fd.Close()

	r, err := rpmpack.NewRPM(rpmpack.RPMMetaData{
		Name:    "velociraptor-client",
		Version: constants.VERSION,
		Release: "A",
		Arch:    "x86_64",
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
			Body:  binary_text,
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

	r.AddPreun(`
/bin/systemctl disable velociraptor_client.service
/bin/systemctl stop velociraptor_client.service
`)

	fd, err = os.OpenFile(*client_rpm_command_output,
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		return fmt.Errorf("Unable to create output file: %w", err)
	}
	defer fd.Close()

	return r.Write(fd)
}

// Systemd based start up scripts (Centos 7, 8)
func doServerRPM() error {
	// Disable logging when creating a deb - we may not create the
	// deb on the same system where the logs should go.
	_ = config.ValidateClientConfig(&config_proto.Config{})

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
	// Debian packages always use the "velociraptor" user.
	config_obj.Frontend.RunAsUser = "velociraptor"
	config_obj.ServerType = "linux"

	var err error

	input := *server_rpm_command_binary
	if input == "" {
		input, err = os.Executable()
		if err != nil {
			return fmt.Errorf("Unable to open executable: %w", err)
		}
	}

	fd, err := os.Open(input)
	if err != nil {
		return fmt.Errorf("Unable to open executable: %w", err)
	}
	defer fd.Close()

	binary_text, err := ioutil.ReadAll(fd)
	if err != nil {
		return fmt.Errorf("Unable to read executable: %w", err)
	}
	fd.Close()

	if len(binary_text) < 4 ||
		binary.LittleEndian.Uint32(binary_text[:4]) != 0x464c457f {
		return fmt.Errorf("Binary does not appear to be an " +
			"ELF binary. Please specify the linux binary " +
			"using the --binary flag.")
	}

	config_file_yaml, err := yaml.Marshal(config_obj)
	if err != nil {
		return err
	}

	r, err := rpmpack.NewRPM(rpmpack.RPMMetaData{
		Name:    "velociraptor-server",
		Version: constants.VERSION,
		Release: "A",
		Arch:    "x86_64",
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
			Body:  binary_text,
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

	output_file := *server_rpm_command_output
	if variant != "" {
		output_file = strings.TrimSuffix(output_file, ".rpm") + variant + ".rpm"
	}

	fmt.Printf("Creating a package for %v\n", output_file)

	fd, err = os.OpenFile(output_file,
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		return fmt.Errorf("Unable to create output file: %w", err)
	}
	defer fd.Close()

	return r.Write(fd)
}

// Simple startup scripts for Sys V based systems (Centos 6)
func doClientSysVRPM() error {
	// Disable logging when creating a deb - we may not create the
	// deb on the same system where the logs should go.
	_ = config.ValidateClientConfig(&config_proto.Config{})

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
			return fmt.Errorf("Unable to open executable: %w", err)
		}
	}

	fd, err := os.Open(input)
	if err != nil {
		return fmt.Errorf("Unable to open executable: %w", err)
	}
	defer fd.Close()

	header := make([]byte, 4)
	_, err = fd.Read(header)
	if err != nil {
		return fmt.Errorf("Unable to read executable: %w", err)
	}

	if binary.LittleEndian.Uint32(header) != 0x464c457f {
		return fmt.Errorf("Binary does not appear to be an " +
			"ELF binary. Please specify the linux binary " +
			"using the --binary flag.")
	}

	_, err = fd.Seek(0, io.SeekStart)
	if err != nil {
		return fmt.Errorf("Unable to read executable: %w", err)
	}

	binary_text, err := ioutil.ReadAll(fd)
	if err != nil {
		return fmt.Errorf("Unable to read executable: %w", err)
	}
	fd.Close()

	r, err := rpmpack.NewRPM(rpmpack.RPMMetaData{
		Name:    "velociraptor",
		Version: constants.VERSION,
		Release: "A",
		Arch:    "x86_64",
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
			Body:  binary_text,
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
/sbin/service velociraptor condrestart > /dev/null 2>&1 || :
`)

	fd, err = os.OpenFile(*client_rpm_command_output,
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		return fmt.Errorf("Unable to  create output file: %w", err)
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
