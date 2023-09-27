#!/bin/bash

function help() {
echo This script will build an offline collector without the GUI.
echo You will need to modify the a spec file with the list of
echo artifacts you want to build.
echo
echo Usage:
echo $0 /path/to/velociraptor /path/to/spec_file.yaml
}

# Path to the VELOCIRAPTOR binary
VELOCIRAPTOR=$1
SPECFILE=$2

if [ ! -f "$VELOCIRAPTOR" ]; then
    echo Please provide a path to the Velociraptor binary.
    echo
    help
    exit -1
fi

if [ ! -f "$SPECFILE" ]; then
    echo Please provide a path to the offline collector spec file.
    echo See https://github.com/Velocidex/velociraptor/tree/master/docs/offline_collector/sample.spec.yaml as an example
    echo
    help
    exit -1
fi


PWD=`pwd`

# Build a server config file with datastore in this directory
JSON_MERGE=$(cat <<EOF
{"Datastore":{"location":"${PWD}", "filestore_directory":"${PWD}"}}
EOF
)

# This will create a temporary server config that will use the current
# directory as a datastore.
$VELOCIRAPTOR config generate --merge "${JSON_MERGE}"  > server.config.yaml

QUERY=$(cat  <<EOF
// this is needed to ensure artifacts are fully loaded before we start
// so their tools are fully registred.
LET _ <= SELECT name FROM artifact_definitions()
LET Spec <= parse_yaml(filename=SPECFILE)
LET _K = SELECT _key FROM items(item=Spec.Artifacts)
SELECT * FROM Artifact.Server.Utils.CreateCollector(
   OS=Spec.OS,
   artifacts=serialize(item=_K._key),
   parameters=serialize(item=Spec.Artifacts),
   target=Spec.Target,
   target_args=Spec.TargetArgs,
   encryption_scheme=Spec.EncryptionScheme,
   opt_verbose=Spec.OptVerbose,
   opt_banner=Spec.OptBanner,
   opt_prompt=Spec.OptPrompt,
   opt_admin=Spec.OptAdmin,
   opt_tempdir=Spec.OptTempdir,
   opt_level=Spec.OptLevel,
   opt_filename_template=Spec.OptFilenameTemplate,
   opt_format=Spec.OptFormat
   )
EOF
)


$VELOCIRAPTOR --config server.config.yaml query -v --env SPECFILE="${SPECFILE}" "${QUERY}" --dump_dir .
