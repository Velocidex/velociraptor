import Alert from 'react-bootstrap/Alert';
import humanizeDuration from "humanize-duration";

const English = {
    SEARCH_CLIENTS: "Search clients",
    "Quarantine description": <>
          <p>You are about to quarantine this host.</p>
          <p>
            While in quarantine, the host will not be able to
            communicate with any other networks, except for the
            Velociraptor server.
          </p>
                              </>,
    "Cannot Quarantine host": "Cannot Quarantine host",
    "Cannot Quarantine host message": (os_name, quarantine_artifact)=>
        <>
          <Alert variant="warning">
            { quarantine_artifact ?
              <p>This Velociraptor instance does not have the <b>{quarantine_artifact}</b> artifact required to quarantine hosts running {os_name}.</p> :
              <p>This Velociraptor instance does not have an artifact name defined to quarantine hosts running {os_name}.</p>
            }
          </Alert>
        </>,
    "Client ID": "Client ID",
    "Agent Version": "Agent Version",
    "Agent Name": "Agent Name",
    "First Seen At": "First Seen At",
    "Last Seen At": "Last Seen At",
    "Last Seen IP": "Last Seen IP",
    "Labels": "Labels",
    "Operating System": "Operating System",
    "Hostname": "Hostname",
    "FQDN": "FQDN",
    "Release": "Release",
    "Architecture": "Architecture",
    "Client Metadata": "Client Metadata",
    "Interrogate": "Interrogate",
    "VFS": "VFS",
    "Collected": "Collected",
    "Unquarantine Host": "Unquarantine Host",
    "Quarantine Host": "Quarantine Host",
    "Add Label": "Add Label",
    "Overview": "Overview",
    "VQL Drilldown": "VQL Drilldown",
    "Shell": "Shell",
    "Close": "Close",
    "Connected": "Connected",
    "time_ago": (value, unit) => {
        return value + ' ' + unit + ' ago';
    },
    "DeleteMessage": "You are about to permanently delete the following clients",
    "You are about to delete": name=>"You are about to delete "+name,
    "Edit Artifact": name=>{
        return "Edit Artifact " + name;
    },
    "Notebook for Collection": name=>"Notebook for Collection "+name,
    "ArtifactDeletionDialog": (session_id, artifacts, total_bytes, total_rows)=>
    <>
      You are about to permanently delete the artifact collection
      <b>{session_id}</b>.
      <br/>
      This collection was the artifacts <b>{artifacts}</b>
      <br/><br/>

      We expect to free up { total_bytes.toFixed(0) } Mb of bulk
      data and { total_rows } rows.
    </>,
    "ArtifactFavorites": artifacts=>
    <>
      You can easily collect the same collection from your
      favorites in future.
      <br/>
      This collection was the artifacts <b>{artifacts}</b>
      <br/><br/>
    </>,
    "DeleteHuntDialog": <>
                    <p>You are about to permanently stop and delete all data from this hunt.</p>
                    <p>Are you sure you want to cancel this hunt and delete the collected data?</p>
                        </>,
    "Notebook for Hunt": hunt_id=>"Notebook for Hunt " + hunt_id,
    "RecursiveVFSMessgae": path=><>
       You are about to recursively fetch all files in <b>{path}</b>.
       <br/><br/>
        This may transfer a large amount of data from the endpoint. The default upload limit is 1gb but you can change it in the Collected Artifacts screen.
     </>,
    "ToolLocalDesc":
    <>
      Tool will be served from the Velociraptor server
      to clients if needed. The client will
      cache the tool on its own disk and compare the hash next
      time it is needed. Tools will only be downloaded if their
      hash has changed.
    </>,
    "ServedFromURL": (base_path, url)=>
    <>
      Clients will fetch the tool directly from
      <a href={base_path + url}>{url}</a> if
      needed. Note that if the hash does not match the
      expected hash the clients will reject the file.
    </>,
    "ServedFromGithub": (github_project, github_asset_regex)=>
    <>
      Tool URL will be refreshed from
      GitHub as the latest release from the project
      <b>{github_project}</b> that matches
      <b>{github_asset_regex}</b>
    </>,
    "PlaceHolder":
    <>
      Tool hash is currently unknown. The first time the tool
      is needed, Velociraptor will download it from it's
      upstream URL and calculate its hash.
    </>,
    "ToolHash":
    <>
      Tool hash has been calculated. When clients need to use
      this tool they will ensure this hash matches what they
      download.
    </>,
    "AdminOverride":
    <>
      Tool was manually uploaded by an
      admin - it will not be automatically upgraded on the
      next Velociraptor server update.
    </>,
    "ToolError":
    <>
      Tool's hash is not known and no URL
      is defined. It will be impossible to use this tool in an
      artifact because Velociraptor is unable to resolve it. You
      can manually upload a file.
    </>,
    "OverrideToolDesc":
    <>
      As an admin you can manually upload a
      binary to be used as that tool. This will override the
      upstream URL setting and provide your tool to all
      artifacts that need it. Alternative, set a URL for clients
      to fetch tools from.
    </>,
    "X per second": x=><>{x} per second</>,
    "HumanizeDuration": difference=>{
        if (difference<0) {
            return <>
                     In {humanizeDuration(difference, {
                         round: true,
                         language: "en",
                     })}
                   </>;
        }
        return <>
                 {humanizeDuration(difference, {
                     round: true,
                     language: "en",
                 })} ago
               </>;
    }
};

export default English;
