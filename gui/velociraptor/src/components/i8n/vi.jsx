import _ from 'lodash';
import hex2a from "./utils";
import React from 'react';
import Alert from 'react-bootstrap/Alert';
import humanizeDuration from "humanize-duration";

import automated from "./vi.json";

const Vietnamese = {
    SEARCH_CLIENTS: "Clients suchen",
    "Quarantine description": (
        <>
          <p>Sie sind dabei, diesen Host unter Quarantäne zu stellen.</p>
          <p>
            Während der Quarantäne kann der Host, außer mit dem Velociraptor-Server, mit keinem andere Netzwerk kommunizieren.
          </p>
        </>),
    "Cannot Quarantine host": "Host kann nicht unter Quarantäne gestellt werden",
    "Cannot Quarantine host message": (os_name, quarantine_artifact)=>
        <>
          <Alert variant="warning">
            { quarantine_artifact ?
              <p>Diese Velociraptor-Instanz verfügt nicht über das Artefakt <b>{quarantine_artifact}</b>, das erforderlich ist, um Hosts unter Quarantäne zu stellen, auf denen {os_name} ausgeführt wird.</p> :
              <p>Für diese Velociraptor-Instanz ist kein Artefaktname definiert, um Hosts unter Quarantäne zu stellen, auf denen {os_name} ausgeführt wird.</p>
            }
          </Alert>
        </>,
    "time_ago": function(value, unit) {
        unit = Vietnamese[unit] || unit;
        return 'Vor ' + value + ' ' + unit;
    },
    "Edit Artifact": name=>{
        return "Artefakt bearbeiten " + name;
    },
    "Notebook for Collection": name=>"Notizbuch für die Sammlung "+name,
    "ArtifactDeletionDialog": (session_id, artifacts, total_bytes, total_rows)=>
    <>
      Sie sind im Begriff, die Artefaktsammlung endgültig zu löschen
      <b>{session_id}</b>.
      <br/>
      Diese Sammlung hatte die Artefakte <b className="wrapped-text">
         {artifacts}
      </b>
      <br/><br/>

      Wir gehen davon aus, { total_bytes.toFixed(0) } MB Massenspeicher freizugeben
      data und { total_rows } Zeilen.
    </>,
    "ArtifactFavorites": artifacts=>
    <>
      Sie können die gleiche Sammlung ganz einfach von Ihrem sammeln
      Favoriten in der Zukunft.
      <br/>
      Diese Sammlung waren die Artefakte <b>{artifacts}</b>
      <br/><br/>
    </>,
    "DeleteHuntDialog": <>
                    <p>You are about to permanently stop and delete all data from this hunt.</p>
                    <p>Are you sure you want to cancel this hunt and delete the collected data?</p>
                        </>,

    "Notebook for Hunt": hunt_id=>"Notizbuch für Hunt " + hunt_id,
    "RecursiveVFSMessage": path=><>
    Sie sind dabei, alle Dateien in <b>{path}</b> rekursiv abzurufen.
    <br/><br/>
    Dadurch können große Datenmengen vom Client übertragen werden. Das Standard-Upload-Limit beträgt 1 GB, aber Sie können es im Bildschirm „Gesammelte Artefakte“ ändern.
    </>,

    "ToolLocalDesc":
    <>
    Das Tool wird bei Bedarf vom Velociraptor-Server an die Clients
    ausgeliefert. Der Client speichert das Tool auf seiner eigenen
    Festplatte und vergleicht den Hash, wenn es das nächste Mal
    benötigt wird. Die Tools werden nur heruntergeladen, wenn sich ihr
    Hash geändert hat.
    </>,
    "ServedFromURL": (base_path, url)=>
    <>
    Die Clients rufen das Tool bei Bedarf direkt von
    <a href={base_path + url}>{url}</a> ab. Wenn der Hashwert nicht mit dem
    erwarteten Hashwert übereinstimmt, weisen die Clients die Datei zurück.
    </>,
    "ServedFromGithub": (github_project, github_asset_regex)=>
    <>
    Die Tool-URL wird von GitHub als die neueste Version des Projekts
    <b>{github_project}</b>, die mit <b>{github_asset_regex}</b>
    übereinstimmt, aktualisiert.
    </>,
    "PlaceHolder":
    <>
    Der Hash-Wert des Tools ist derzeit unbekannt. Das erste Mal,
    wenn das Tool benötigt wird, lädt Velociraptor es von seiner
    Upstream-URL herunter und berechnet den Hash.
    </>,
    "ToolHash":
    <>
    Der Hashwert des Tools wurde berechnet. Wenn Clients dieses Tool
    verwenden müssen, stellt es sicher, dass dieser Hash mit dem herunter
    geladenen Binary übereinstimmt.
    </>,
    "AdminOverride":
    <>
    Das Tool wurde manuell von einem Administrator hochgeladen -
    es wird beim nächsten Velociraptor-Server-Update nicht
    automatisch aktualisiert.
    </>,
    "ToolError":
    <>
    Der Hash des Tools ist nicht bekannt und es ist keine URL definiert.
    Es wird unmöglich sein, dieses Tools in einem Artefakt zu verwenden,
    da Velociraptor nicht in der Lage ist, es aufzulösen.
    Sie können eine Datei manuell hochladen.
    </>,
    "OverrideToolDesc":
    <>
    Als Administrator können Sie manuell eine Binärdatei hochladen, die als
    dieses Tool verwendet werden soll. Dadurch wird die Upstream-URL-Einstellung
    außer Kraft gesetzt und Ihr Tool allen Artefakten zur Verfügung gestellt,
    die es benötigen. Alternativ können Sie eine URL festlegen, von der die
    Clients die Tools abrufen können.
    </>,

    "X per second": x=><>{x} pro Sekunde</>,
    "HumanizeDuration": difference=>{
        if (difference<0) {
            return <>
                     In {humanizeDuration(difference, {
                         round: true,
                         language: "de",
                     })}
                   </>;
        }
        return <>
                 Vor {humanizeDuration(difference, {
                     round: true,
                     language: "de",
                 })}
               </>;
    },
    "EventMonitoringCard":
    <>
    Die Event-Monitoring zielt auf bestimmte Labelgruppen ab. Wählen Sie oben
    eine Kennzeichnungsgruppe aus, um spezifische Ereignisartefakte für
    diese Gruppe zu konfigurieren.
    </>,
    "TablePagination": (from, to, size)=>
    <>Zeigt Zeile { from } bis { to } von { size }</>,

    // Below need verification
    "Role_administrator": "Server-Administrator",
     "Role_org_admin": "Organisationsadministrator",
     "Role_reader": "Nur-Lese-Benutzer",
     "Role_analyst": "Analyst",
     "Role_investigator": "Ermittler",
     "Role_artifact_writer": "Artefaktschreiber",
     "Role_api": "Schreibgeschützter API-Client",

    "Perm_ALL_QUERY": "Alle Abfragen",
     "Perm_ANY_QUERY": "Jede Abfrage",
     "Perm_PUBISH": "Veröffentlichen",
     "Perm_READ_RESULTS" : "Ergebnisse lesen",
     "Perm_LABEL_CLIENT": "Label-Clients",
     "Perm_COLLECT_CLIENT": "Kunden sammeln",
     "Perm_START_HUNT": "Jagd starten",
     "Perm_COLLECT_SERVER": "Server sammeln",
     "Perm_ARTIFACT_WRITER": "Artefaktschreiber",
     "Perm_SERVER_ARTIFACT_WRITER": "Serverartefakt-Schreiber",
     "Perm_EXECVE": "EXECVE",
     "Perm_NOTEBOOK_EDITOR": "Notizbuch-Editor",
     "Perm_SERVER_ADMIN": "Server-Admin",
     "Perm_ORG_ADMIN": "Org-Admin",
     "Perm_IMPERSONATION": "Identitätswechsel",
     "Perm_FILESYSTEM_READ": "Dateisystem lesen",
     "Perm_FILESYSTEM_WRITE": "Dateisystem schreiben",
     "Perm_MACHINE_STATE": "Maschinenstatus",
     "Perm_PREPARE_RESULTS": "Ergebnisse vorbereiten",
     "Perm_DATASTORE_ACCESS": "Datenspeicherzugriff",


     "ToolPerm_ALL_QUERY" : "Alle Abfragen uneingeschränkt ausgeben",
     "ToolPerm_ANY_QUERY": "Jede Abfrage überhaupt ausgeben (AllQuery impliziert AnyQuery)",
     "ToolPerm_PUBISH": "Ereignisse in serverseitigen Warteschlangen veröffentlichen (normalerweise nicht erforderlich)",
     "ToolPerm_READ_RESULTS": "Ergebnisse von bereits ausgeführten Jagden, Flows oder Notizbüchern lesen",
     "ToolPerm_LABEL_CLIENT": "Kann Client-Labels und Metadaten manipulieren",
     "ToolPerm_COLLECT_CLIENT": "Planen oder stornieren Sie neue Sammlungen für Kunden",
     "ToolPerm_START_HUNT": "Neue Jagd starten",
     "ToolPerm_COLLECT_SERVER": "Neue Artefaktsammlungen auf Velociraptor-Servern planen",
     "ToolPerm_ARTIFACT_WRITER": "Benutzerdefinierte Artefakte hinzufügen oder bearbeiten, die auf dem Server ausgeführt werden",
     "ToolPerm_SERVER_ARTIFACT_WRITER": "Benutzerdefinierte Artefakte hinzufügen oder bearbeiten, die auf dem Server ausgeführt werden",
     "ToolPerm_EXECVE": "Dürfen beliebige Befehle auf Clients ausführen",
     "ToolPerm_NOTEBOOK_EDITOR": "Erlaubt, Notizbücher und Zellen zu ändern",
     "ToolPerm_SERVER_ADMIN": "Zur Verwaltung der Serverkonfiguration berechtigt",
     "ToolPerm_ORG_ADMIN" : "Zur Verwaltung von Organisationen berechtigt",
     "ToolPerm_IMPERSONATION": "Erlaubt dem Benutzer, einen anderen Benutzernamen für das query()-Plugin anzugeben",
     "ToolPerm_FILESYSTEM_READ": "Erlaubt, beliebige Dateien aus dem Dateisystem zu lesen",
     "ToolPerm_FILESYSTEM_WRITE": "Darf Dateien im Dateisystem erstellen",
     "ToolPerm_MACHINE_STATE": "Erlaubt, Zustandsinformationen von Maschinen zu sammeln (z. B. pslist())",
     "ToolPerm_PREPARE_RESULTS": "Darf ZIP-Dateien erstellen",
     "ToolPerm_DATASTORE_ACCESS": "Zugriff auf Rohdatenspeicher erlaubt",

};

_.each(automated, (v, k)=>{
    Vietnamese[hex2a(k)] = v;
});

export default Vietnamese;
