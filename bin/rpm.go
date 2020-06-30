package main

import (
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/Velocidex/yaml/v2"
	"github.com/google/rpmpack"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
)

var (
	rpm_command = app.Command(
		"rpm", "Create an rpm package")

	client_rpm_command = rpm_command.Command(
		"client", "Create a client package from a server config file.")

	client_rpm_command_use_sysv = client_rpm_command.Flag(
		"use_sysv", "Use sys V style services (Centos 6)").Bool()

	client_rpm_command_output = client_rpm_command.Flag(
		"output", "Filename to output").Default(
		fmt.Sprintf("velociraptor_%s_client.rpm", constants.VERSION)).
		String()

	client_rpm_command_binary = client_rpm_command.Flag(
		"binary", "The binary to package").String()

	rpm_client_service_definition = `
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
	$VELOCIRAPTOR --config "$VELOCIRAPTOR_CONFIG" frontend  && success || failure
	RETVAL=$?
	[ $RETVAL -eq 0 ] && touch $lockfile
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
func doClientRPM() {
	// Disable logging when creating a deb - we may not create the
	// deb on the same system where the logs should go.
	config.ValidateClientConfig(&config_proto.Config{})

	config_obj, err := DefaultConfigLoader.
		WithRequiredClient().LoadAndValidate()
	kingpin.FatalIfError(err, "Unable to load config file")

	config_file_yaml, err := yaml.Marshal(getClientConfig(config_obj))
	kingpin.FatalIfError(err, "marshal")

	input := *client_rpm_command_binary

	if input == "" {
		input, err = os.Executable()
		kingpin.FatalIfError(err, "Unable to open executable")
	}

	fd, err := os.Open(input)
	kingpin.FatalIfError(err, "Unable to open executable")
	defer fd.Close()

	header := make([]byte, 4)
	fd.Read(header)
	if binary.LittleEndian.Uint32(header) != 0x464c457f {
		kingpin.Fatalf("Binary does not appear to be an " +
			"ELF binary. Please specify the linux binary " +
			"using the --binary flag.")
	}

	fd.Seek(0, os.SEEK_SET)

	binary_text, err := ioutil.ReadAll(fd)
	kingpin.FatalIfError(err, "Unable to open executable")
	fd.Close()

	r, err := rpmpack.NewRPM(rpmpack.RPMMetaData{
		Name:    "velociraptor-client",
		Version: constants.VERSION,
		Release: "A",
		Arch:    "x86_64",
	})
	kingpin.FatalIfError(err, "Unable to create RPM")

	r.AddFile(
		rpmpack.RPMFile{
			Name:  "/etc/velociraptor/client.config.yaml",
			Mode:  0600,
			Body:  config_file_yaml,
			Owner: "velociraptor",
			Group: "velociraptor",
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
			Mode:  0755,
			Owner: "root",
			Group: "root",
		})

	r.AddPostin(`/bin/systemctl enable velociraptor_client
/bin/systemctl start velociraptor_client
`)

	r.AddPreun(`
/bin/systemctl disable velociraptor_client
/bin/systemctl stop velociraptor_client
`)

	fd, err = os.OpenFile(*client_rpm_command_output,
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	kingpin.FatalIfError(err, "Unable to create output file")
	defer fd.Close()

	err = r.Write(fd)
	kingpin.FatalIfError(err, "Unable to write output file")
}

// Simple startup scripts for Sys V based systems (Centos 6)
func doClientSysVRPM() {
	// Disable logging when creating a deb - we may not create the
	// deb on the same system where the logs should go.
	config.ValidateClientConfig(&config_proto.Config{})

	config_obj, err := DefaultConfigLoader.
		WithRequiredClient().LoadAndValidate()
	kingpin.FatalIfError(err, "Unable to load config file")

	config_file_yaml, err := yaml.Marshal(getClientConfig(config_obj))
	kingpin.FatalIfError(err, "marshal")

	input := *client_rpm_command_binary

	if input == "" {
		input, err = os.Executable()
		kingpin.FatalIfError(err, "Unable to open executable")
	}

	fd, err := os.Open(input)
	kingpin.FatalIfError(err, "Unable to open executable")
	defer fd.Close()

	header := make([]byte, 4)
	fd.Read(header)
	if binary.LittleEndian.Uint32(header) != 0x464c457f {
		kingpin.Fatalf("Binary does not appear to be an " +
			"ELF binary. Please specify the linux binary " +
			"using the --binary flag.")
	}

	fd.Seek(0, os.SEEK_SET)

	binary_text, err := ioutil.ReadAll(fd)
	kingpin.FatalIfError(err, "Unable to open executable")
	fd.Close()

	r, err := rpmpack.NewRPM(rpmpack.RPMMetaData{
		Name:    "velociraptor",
		Version: constants.VERSION,
		Release: "A",
		Arch:    "x86_64",
	})
	kingpin.FatalIfError(err, "Unable to create RPM")
	r.AddFile(
		rpmpack.RPMFile{
			Name:  "/etc/velociraptor/client.config.yaml",
			Mode:  0600,
			Body:  config_file_yaml,
			Owner: "velociraptor",
			Group: "velociraptor",
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
			Body:  []byte(rpm_client_service_definition),
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
	kingpin.FatalIfError(err, "Unable to create output file")
	defer fd.Close()

	err = r.Write(fd)
	kingpin.FatalIfError(err, "Unable to write output file")
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case client_rpm_command.FullCommand():
			if *client_rpm_command_use_sysv {
				doClientSysVRPM()
			} else {
				doClientRPM()
			}

		default:
			return false
		}
		return true
	})
}
