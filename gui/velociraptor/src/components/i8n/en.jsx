import React from 'react';
import Alert from 'react-bootstrap/Alert';
import humanizeDuration from "humanize-duration";
import api from '../core/api-service.jsx';

const English = {
    "SEARCH_CLIENTS": "Search clients",
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
    "Search": "Search",
    "Main Menu": "Main Menu",
    "Toggle Main Menu": "Toggle Main Menu",
    "Welcome": "Welcome",
    "Connected": "Connected",
    "time_ago": (value, unit) => {
        return value + ' ' + unit + ' ago';
    },
    "DeleteMessage": "You are about to permanently delete the following clients",
    "KillMessage": "You are about to kill the following clients",
    "You are about to delete": name=>"You are about to delete "+name,
    "Edit Artifact": name=>{
        return "Edit Artifact " + name;
    },
    "Notebook for Collection": name=>"Notebook for Collection "+name,
    "Import Artifacts": length=><>Import {length} Artifacts</>,
    "ArtifactDeletionDialog": (session_id, artifacts, total_bytes, total_rows)=>
    <>
      You are about to permanently delete the artifact
      collection <b>{session_id}</b>.
      <br/>
      This collection had the artifacts <b className="wrapped-text">
                                          {artifacts}</b>
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
    "RecursiveVFSMessage": path=><>
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
    "ServedFromURL": (url)=>
    <>
      Clients will fetch the tool directly
      from <a href={api.href(url)}>{url}</a> if
      needed. Note that if the hash does not match the
      expected hash the clients will reject the file.
    </>,
    "ServedFromGithub": (github_project, github_asset_regex)=>
    <>
      Tool URL will be refreshed from
      GitHub as the latest release from the
      project <b>{github_project}</b> that
      matches <b>{github_asset_regex}</b>
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
    },
    "EventMonitoringCard":
    <>
      Event monitoring targets specific label groups.
      Select a label group above to configure specific
      event artifacts targetting that group.
    </>,
    "Deutsch": "German",
    "_ts": "ServerTime",
    "TablePagination": (from, to, size)=>
    <>{ from.toString() }-{ to.toString() }/{ size.toString() }</>,
    "Verified Email" : "Verified Email",
    "Account Locked" : "Account Locked",
    "Role_administrator" : "Server Administrator",
    "Role_org_admin" : "Organization Administrator",
    "Role_reader" : "Read-Only User",
    "Role_analyst" : "Analyst",
    "Role_investigator" : "Investigator",
    "Role_artifact_writer" : "Artifact Writer",
    "Role_api" : "API Client",
    "ToolRole_administrator" :
    <>
    Like any system, Velociraptor needs an administrator which is all powerful. This account can run arbitrary VQL on the server, reconfigure the server, etc.  The ability to add/create/edit/remove users is dependent on the organizations to which this account belongs.
    </>,
    "ToolRole_org_admin" :
    <>
    This role provides the ability to manage organizations.  It would typically be used together with another role.
    </>,
    "ToolRole_reader" :
    <>
    This role provides the ability to read previously collected results but does not allow the user to actually make any changes.  This role is useful to give unpriviledged users visibility into what information is being collected without giving them access to modify anything.
    </>,
    "ToolRole_analyst" :
    <>
    This role provides the ability to read existing collected data and also run some server side VQL in order to do post processing of this data or annotate it. Analysts typically use the notebook or download collected data offline for post processing existing hunt data. Analysts may not actually start new collections or hunts themselves.
    </>,
    "ToolRole_investigator" :
    <>
    This role provides the ability to read existing collected data and also run some server side VQL in order to do post processing of this data or annotate it. Investigators typically use the notebook or download collected data offline for post processing existing hunt data. Investigators may start new collections or hunts themselves.
    </>,
    "ToolRole_artifact_writer" :
    <>
    This role allows a user to create or modify new client side artifacts (They are not able to modify server side artifacts). This user typically has sufficient understanding and training in VQL to write flexible artifacts. Artifact writers are very powerful as they can easily write a malicious artifact and collect it on the endpoint. Therefore they are equivalent to domain admins on endpoints. You should restrict this role to very few people.
    </>,
    "ToolRole_api" :
    <>
    This role is required to provide the ability for the user to connect over the API port.
    </>,

    "Perm_ANY_QUERY" : "Any Query",
    "Perm_PUBISH" : "Publish",
    "Perm_READ_RESULTS" : "Read results",
    "Perm_LABEL_CLIENT" : "Label Clients",
    "Perm_COLLECT_CLIENT" : "Collect Client",
    "Perm_COLLECT_BASIC": "Collect Basic Client",
    "Perm_START_HUNT" : "Start Hunt",
    "Perm_COLLECT_SERVER" : "Collect Server",
    "Perm_ARTIFACT_WRITER" : "Artifact Writer",
    "Perm_SERVER_ARTIFACT_WRITER" : "Server Artifact Writer",
    "Perm_EXECVE" : "EXECVE",
    "Perm_NOTEBOOK_EDITOR" : "Notebook Editor",
    "Perm_SERVER_ADMIN" : "Server Admin",
    "Perm_ORG_ADMIN" : "Org Admin",
    "Perm_IMPERSONATION" : "Impersonation",
    "Perm_FILESYSTEM_READ" : "Filesystem Read",
    "Perm_FILESYSTEM_WRITE" : "Filesystem Write",
    "Perm_NETWORK": "Network Access",
    "Perm_MACHINE_STATE" : "Machine State",
    "Perm_PREPARE_RESULTS" : "Prepare Results",
    "Perm_DELETE_RESULTS" : "Delete Results",
    "Perm_DATASTORE_ACCESS" : "Datastore Access",


    "ToolPerm_ANY_QUERY" : "Issue any query at all ",
    "ToolPerm_PUBISH" : "Publish events to server side queues (typically not needed)",
    "ToolPerm_READ_RESULTS" : "Read results from already run hunts, flows, or notebooks",
    "ToolPerm_LABEL_CLIENT" : "Can manipulate client labels and metadata",
    "ToolPerm_COLLECT_CLIENT" : "Schedule or cancel new collections on clients",
    "ToolPerm_COLLECT_BASIC" : "Schedule basic collections on clients",
    "ToolPerm_START_HUNT" : "Start a new hunt",
    "ToolPerm_COLLECT_SERVER" : "Schedule new artifact collections on Velociraptor servers",
    "ToolPerm_ARTIFACT_WRITER" : "Add or edit custom artifacts that run on the server",
    "ToolPerm_SERVER_ARTIFACT_WRITER" : "Add or edit custom artifacts that run on the server",
    "ToolPerm_EXECVE" : "Allowed to execute arbitrary commands on clients",
    "ToolPerm_NOTEBOOK_EDITOR" : "Allowed to change notebooks and cells",
    "ToolPerm_SERVER_ADMIN" : "Allowed to manage server configuration",
    "ToolPerm_ORG_ADMIN" : "Allowed to manage organizations",
    "ToolPerm_IMPERSONATION" : "Allows the user to specify a different username for the query() plugin",
    "ToolPerm_FILESYSTEM_READ" : "Allowed to read arbitrary files from the filesystem",
    "ToolPerm_FILESYSTEM_WRITE" : "Allowed to create files on the filesystem",
    "ToolPerm_NETWORK": "Allowed to run plugins that make network connections",
    "ToolPerm_MACHINE_STATE" : "Allowed to collect state information from machines (e.g. pslist())",
    "ToolPerm_PREPARE_RESULTS" : "Allowed to create zip files",
    "ToolPerm_DELETE_RESULTS" : "Allowed to delete clients, flows and other data",
    "ToolPerm_DATASTORE_ACCESS" : " Allowed raw datastore access",

    "ToolUsernamePasswordless" :
    <>
        This server is configured to authenticate users using an external authentication host.  This account must exist on the authentication system for login to be successful.
    </>,
    "Add User" : "Add User",
    "Update User" : "Update User",
    "Roles" : "Roles",
    "Roles in": org => {
    return "Roles in " + org;
    },
    "Not a member of": org => {
        return "Not a member of " + org;
    },

    "ToolRoleBasedPermissions" :
    <>
    Role-Based Permissions allow the administrator to grant sets of permissions for common activities.  A user may have multiple roles assigned.
    </>,
    "ToolEffectivePermissions" :
    <>
    Roles are defined as sets of fine-grained permissions.  This is the current set of permissions defined by the roles for this user.
    </>,
    "ToolOrganizations" :
    <>
    Organizations allow multiple tenants to use this Velociraptor server.  If a user is not assigned to an organization, it is a member of the Organizational Root, which implies membership in all organizations.
    </>,
    "User does not exist": (username)=><>User {username} does not exist.</>,
    "Do you want to delete?": (username)=>"Do you want to delete " + username + "?",
    "WARN_REMOVE_USER_FROM_ORG": (user, org)=>(
        <>
          You are about to remove user {user} from Org {org} <br/>
        </>),
    "RUNNING": "Scheduled",
    "WAITING": "Waiting to run",
    "IN_PROGRESS": "In Progress",
    "UNRESPONSIVE": "Unresponsive",
    "FINISHED": "Completed",
    "ERROR": "Error",
};

export default English;
