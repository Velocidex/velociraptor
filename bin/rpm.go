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

	server_rpm_command = rpm_command.Command(
		"server", "Create a server package from a server config file.")

	server_rpm_command_output = server_rpm_command.Flag(
		"output", "Filename to output").Default(
		fmt.Sprintf("velociraptor_%s_server.rpm", constants.VERSION)).
		String()

	server_rpm_command_binary = server_rpm_command.Flag(
		"binary", "The binary to package").String()

	rpm_server_service_definition = `
#!/bin/bash
#
# velociraptor_server		Start up the Velociraptor server daemon
#
# chkconfig: 2345 55 25
# description: Velociraptor is an endpoint monitoring tool
#
# processname: velociraptor
# config: /etc/velociraptor/server.config.yaml
# pidfile: /var/run/velociraptor_server.pid

### BEGIN INIT INFO
# Provides: velociraptor_server
# Required-Start: $local_fs $network $syslog
# Required-Stop: $local_fs $syslog
# Should-Start: $syslog
# Should-Stop: $network $syslog
# Default-Start: 2 3 4 5
# Default-Stop: 0 1 6
# Short-Description: Start up the Velociraptor server daemon
### END INIT INFO

# source function library
. /etc/rc.d/init.d/functions

RETVAL=0
prog="velociraptor"
lockfile=/var/lock/subsys/$prog
VELOCIRAPTOR=/usr/local/bin/velociraptor
VELOCIRAPTOR_CONFIG=/etc/velociraptor/server.config.yaml
PID_FILE=/var/run/velociraptor_server.pid

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
	status -p $PID_FILE velociraptor_server
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

func doServerRPM() {
	// Disable logging when creating a deb - we may not create the
	// deb on the same system where the logs should go.
	config.ValidateClientConfig(&config_proto.Config{})

	config_obj, err := DefaultConfigLoader.WithRequiredFrontend().LoadAndValidate()
	kingpin.FatalIfError(err, "Unable to load config file")

	// Debian packages always use the "velociraptor" user.
	config_obj.Frontend.RunAsUser = "velociraptor"
	config_obj.ServerType = "linux"

	config_file_yaml, err := yaml.Marshal(config_obj)
	kingpin.FatalIfError(err, "marshal")

	input := *server_rpm_command_binary

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

	binary_text, err := ioutil.ReadAll(fd)
	kingpin.FatalIfError(err, "Unable to open executable")
	fd.Close()

	r, err := rpmpack.NewRPM(rpmpack.RPMMetaData{
		Name:    "velociraptor-server",
		Version: constants.VERSION,
		Release: "A",
		Arch:    "amd64",
	})
	kingpin.FatalIfError(err, "Unable to create RPM")
	r.AddFile(
		rpmpack.RPMFile{
			Name:  "/etc/velociraptor/server.config.yaml",
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
			Name:  "/etc/rc.d/init.d/velociraptor_server",
			Body:  []byte(rpm_server_service_definition),
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

	r.AddPostin("/sbin/chkconfig --add velociraptor_server")
	r.AddPreun(`
if [ "$1" = 0 ]
then
        /sbin/service velociraptor_server stop > /dev/null 2>&1 || :
        /sbin/chkconfig --del velociraptor_server
fi
`)
	r.AddPostun(`
/sbin/service velociraptor_server condrestart > /dev/null 2>&1 || :
`)

	fd, err = os.OpenFile(*server_rpm_command_output,
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	kingpin.FatalIfError(err, "Unable to create output file")
	defer fd.Close()

	err = r.Write(fd)
	kingpin.FatalIfError(err, "Unable to write output file")
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case server_rpm_command.FullCommand():
			doServerRPM()

		default:
			return false
		}
		return true
	})
}
