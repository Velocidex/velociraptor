import _ from 'lodash';
import hex2a from "./utils";
import React from 'react';
import Alert from 'react-bootstrap/Alert';
import humanizeDuration from "humanize-duration";
import api from '../core/api-service.jsx';

import automated from "./de.json";

const Deutsch = {
    "SEARCH_CLIENTS": "Clients suchen",
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
    "Client ID": "Client ID",
    "Agent Version": "Agent Version",
    "Agent Name": "Name des Agentens",
    "First Seen At": "Zuerst gesehen bei",
    "Last Seen At": "Zuletzt gesehen bei",
    "Last Seen IP": "Zuletzt gesehene IP",
    "Labels": "Label",
    "Operating System": "Betriebssystem",
    "Hostname": "Hostname",
    "FQDN": "FQDN",
    "Release": "Veröffentlichung",
    "Architecture": "Architektur",
    "Client Metadata": "Client-Metadaten",
    "Interrogate": "Abfragen",
    "VFS": "Virtuelles Dateisystem",
    "Collected": "Gesammelt",
    "Unquarantine Host": "Hostquarantäne aufheben",
    "Quarantine Host": "Quarantäne-Host",
    "Quarantine Message": "Quarantänenachricht",
    "Add Label": "Label hinzufügen",
    "Overview": "Überblick",
    "VQL Drilldown": "VQL Detailanalyse",
    "Shell": "Shell",
    "Close": "Schließen",
    "Connected": "Verbunden",
    "seconds": "Sekunden",
    "minutes": "Minuten",
    "hours": "Stunden",
    "days": "Tagen",
    "time_ago": function(value, unit) {
        unit = Deutsch[unit] || unit;
        return 'Vor ' + value + ' ' + unit;
    },
    "Online": "Online",
    "Label Clients": "Client Label",
    "Existing": "Vorhandenen",
    "A new label": "Ein neues Label",
    "Add it!": "Hinzufügen!",
    "Delete Clients": "Clients löschen",
    "DeleteMessage": "Sie sind im Begriff, die folgenden Clients endgültig zu löschen",
    "Yeah do it!": "Ja, mach es!",
    "Goto Page": "Gehe zu Seite",
    "Table is Empty": "Die Tabelle ist leer",
    "OS Version": "Version des Betriebssystems",
    "Select a label": "Wählen Sie ein Label aus",
    "Expand": "Aufklappen",
    "Collapse": "Zuklappen",
    "Hide Output": "Ausgaben ausblenden",
    "Load Output": "Ausgaben laden",
    "Stop": "Stop",
    "Delete": "Löschen",
    "Run command on client": "Befehl auf dem Client ausführen",
    "Type VQL to run on the client": "Geben Sie VQL ein, das auf dem Client ausgeführt werden soll",
    "Run VQL on client": "Führen Sie VQL auf dem Client aus",
    "Artifact details": "Details des Artefaktes",
    "Artifact Name": "Name des Artefaktes",
    "Upload artifacts from a Zip pack": "Laden Sie Artefakte aus einem Zip-Paket hoch",
    "Select artifact pack (Zip file with YAML definitions)": "Artefaktpaket auswählen (Zip-Datei mit YAML-Definitionen)",
    "Click to upload artifact pack file": "Klicken Sie hier, um die Artefaktpaketdatei hochzuladen",
    "Delete an artifact": "Löschen Sie ein Artefakt",
    "You are about to delete": name=>"Sie sind im Begriff " + name + " zu löschen",
    "Add an Artifact": "Hinzufügen eines Artefaktes",
    "Edit an Artifact": "Bearbeiten eines Artefaktes",
    "Delete Artifact": "Artefakt löschen",
    "Hunt Artifact": "Artefakt suchen",
    "Collect Artifact": "Artefakte einsammeln",
    "Upload Artifact Pack": "Artefaktpaket hochladen",
    "Search for artifact": "Artefakte suchen",
    "Search for an artifact to view it": "Suchen Sie nach einem Artefakt, um es anzuzeigen",
    "Edit Artifact": name=>{
        return "Artefakt bearbeiten " + name;
    },
    "Create a new artifact": "Erstellen eines neuen Artefakt",
    "Save": "Speichern",
    "Search": "Suchen",
    "Toggle Main Menu": "Hauptmenü umschalten",
    "Main Menu": "Hauptmenü",
    "Welcome": "Willkommen",

    // Keyboard navigation.
    "Global hotkeys": "Globale Hotkeys",
    "Goto dashboard": "Gehe zum Dashboard",
    "Collected artifacts": "Gesammelte Artefakte",
    "Show/Hide keyboard hotkeys help": "Hilfe zum Ein-/Ausblenden von Tastatur-Hotkeys",
    "Focus client search box": "Fokus-Client-Suchfeld",
    "New Artifact Collection Wizard": "Wizard zum Sammeln neuer Artefakte",
    "Artifact Selection Step": "Artefaktauswahlschritt",
    "Parameters configuration Step": "Parameterkonfigurationsschritt",
    "Collection resource specification": "Spezifikation der Erfassungsressourcen",
    "Launch artifact": "Artefakt starten",
    "Go to next step": "Nächster Schritt",
    "Go to previous step": "Vorheriger Schritt",
    "Select next collection": "Wählen Sie die nächste Sammlung aus",
    "Select previous collection": "Vorherige Sammlung auswählen",
    "View selected collection results": "Zeigen Sie ausgewählte Sammlungsergebnisse an",
    "View selected collection overview": "Ausgewählte Sammlungsübersicht anzeigen",
    "View selected collection logs": "Zeigen Sie ausgewählte Sammlungsprotokolle an",
    "View selected collection uploaded files": "Zeigen Sie ausgewählte hochgeladene Sammlungsdateien an",
    "Editor shortcuts": "Editor-Shortcuts",
    "Popup the editor configuration dialog": "Öffnen Sie den Konfigurationsdialog des Editors",
    "Save editor contents": "Editorinhalte speichern",
    "Keyboard shortcuts": "Tastaturkürzel",
    "Yes do it!": "Ja, mach das!",
    "Inspect Raw JSON": "Raw JSON prüfen",
    "Raw Response JSON": "Raw Response JSON",
    "Show/Hide Columns": "Spalten ein-/ausblenden",
    "Set All": "Alles setzen",
    "Clear All": "Alles löschen",
    "Exit Fullscreen": "Vollbild beenden",
    "Artifact Collection": "Sammlung von Artefakten",
    "Uploaded Files": "Hochgeladene Dateien",
    "Results": "Ergebnisse",
    "Flow Details": "Flow-Details",
    "Notebook for Collection": name=>"Notizbuch für die Sammlung "+name,
    "Please click a collection in the above table":"Bitte klicken Sie auf eine Sammlung in der obigen Tabelle",
    "Artifact Names": "Artefaktnamen",
    "Creator": "Creator",
    "Create Time": "Creation Zeit ",
    "Start Time": "Startzeit",
    "Last Active": "Letzte Aktivität",
    "Duration": "Dauer",
    " Running...": " Betrieb...",
    "State": "Zustand",
    "Error": "Fehler",
    "CPU Limit": "CPU-Limit",
    "IOPS Limit": "IOPS-Beschränkungen",
    "Timeout": "Timeout",
    "Max Rows": "Max. Zeilen",
    "Max MB": "Max MB",
    "Artifacts with Results": "Artefakte mit Ergebnissen",
    "Total Rows": "Gesamtzeilen",
    "Uploaded Bytes": "Hochgeladene Bytes",
    "Files uploaded": "Hochgeladene Dateien",
    "Download Results": "Ergebnisse herunterladen",
    "Set a password in user preferences to lock the download file.": "Legen Sie in den Benutzereinstellungen ein Passwort fest, um die Download-Datei zu sichern.",
    "Prepare Download": "Download vorbereiten",
    "Prepare Collection Report": "SammlungsReport erstellen",
    "Available Downloads": "Verfügbare Downloads",
    "Size (Mb)": "Größe (MB)",
    "Date": "Datum",
    "Unlimited": "Unbegrenzt",
    "rows": "Reihen",
    "Request sent to client": "Anfrage an Client gesendet",
    "Description": "Beschreibung",
    "Created": "Erstellt",
    "Manually add collection to hunt": "Sammlung manuell zur Hunt hinzufügen",
    "No compatible hunts.": "Keine kompatiblen Hunts.",
    "Please create a hunt that collects one or more of the following artifacts.":"Bitte erstellen Sie eine Hunt, die eines oder mehrere der folgenden Artefakte sammelt.",
    "Requests": "Anfragen",
    "Notebook": "Notizbuch",
    "Permanently delete collection": "Sammlung endgültig löschen",
    "ArtifactDeletionDialog": (session_id, artifacts, total_bytes, total_rows)=>
    <>
      Sie sind im Begriff, die Artefaktsammlung endgültig zu
      löschen <b>{session_id}</b>.
      <br/>
      Diese Sammlung hatte die Artefakte <b className="wrapped-text">
         {artifacts}
      </b>
      <br/><br/>

      Wir gehen davon aus, { total_bytes.toFixed(0) } MB Massenspeicher freizugeben
      data und { total_rows } Zeilen.
    </>,
    "Save this collection to your Favorites": "Speichern Sie diese Sammlung in Ihren Favoriten",
    "ArtifactFavorites": artifacts=>
    <>
      Sie können die gleiche Sammlung ganz einfach von Ihrem sammeln
      Favoriten in der Zukunft.
      <br/>
      Diese Sammlung waren die Artefakte <b>{artifacts}</b>
      <br/><br/>
    </>,
    "New Favorite name": "Neuer Name für Favoriten",
    "Describe this favorite": "Beschreibe diesen Favoriten",
    "New Collection": "Neue Kollektion",
    "Add to hunt": "Zur Hunt hinzufügen",
    "Delete Artifact Collection": "Artefaktsammlung löschen",
    "Cancel Artifact Collection": "Artefaktsammlung abbrechen",
    "Copy Collection": "Sammlung kopieren",
    "Save Collection": "Sammlung speichern",
    "Build offline collector": "Offline-Collector erstellen",
    "Notebooks": "Notizbücher",
    "Full Screen": "Ganzer Bildschirm",
    "Delete Notebook": "Notizbuch löschen",
    "Notebook Uploads": "Notizbuch Uploads",
    "Export Notebook": "Notizbuch exportieren",
    "FINISHED": "FERTIG",
    "RUNNING": "LÄUFT",
    "STOPPED": "GESTOPPT",
    "PAUSED": "ANGEHALTEN",
    "ERROR": "Fehler",
    "INFO": "Info",
    "DEBUG": "Debug",
    "CANCELLED": "Abgebrochen",
    "Search for artifacts...": "Suche nach Artefakten...",
    "Favorite Name": "Name des Favoriten",
    "Artifact": "Artefakt",
    "No artifacts configured. Please add some artifacts to collect": "Keine Artefakte konfiguriert. Bitte fügen Sie einige Artefakte zum Sammelung hinzu",

    "Artifacts": "Artefakte",
    "Collected Artifacts": "Gesammelte Artefakte",
    "Flow ID": "Flow-ID",
    "FlowId": "Flow-ID",
    "Goto notebooks": "Gehe zu Notizbüchern",
    "Max Mb": "Max Mb",
    "Mb": "Mb",
    "Name": "Name",
    "Ops/Sec": "Operationen/Sek",
    "Rows": "Zeilen",
    "New Collection: Select Artifacts to collect":"Neue Sammlung: Artefakte zum Sammeln auswählen",
    "Select Artifacts":"Artefakte auswählen",
    "Configure Parameters":"Parameter konfigurieren",
    "Specify Resources":"Ressourcen festlegen",
    "Review":"Review",
    "Launch":"Starten",
    "New Collection: Configure Parameters":"Neue Sammlung: Parameter konfigurieren",
    "New Collection: Specify Resources":"Neue Sammlung: Ressourcen angeben",
    "New Collection: Review request":"Neue Sammlung: Anfrage prüfen",
    "New Collection: Launch collection":"Neue Kollektion: Kollektion starten",

    "CPU Limit Percent":"CPU Limit (Prozent)",
    "IOps/Sec":"IOps/Sec",
    "Max Execution Time in Seconds":"Max. Ausführungszeit in Sekunden",
    "Max Idle Time in Seconds":"Maximale Leerlaufzeit in Sekunden",
    "If set collection will be terminated after this many seconds with no progress.":"Wenn die Satzsammlung nach so vielen Sekunden ohne Fortschritt beendet wird.",
    "Max bytes Uploaded":"Max. MB hochgeladen",
    "Collection did not upload files":"Sammlung hat keine Dateien hochgeladen",

    "Create Offline collector: Select artifacts to collect":"Offline-Collector erstellen: Zu sammelnde Artefakte auswählen",
    "Configure Collection":"Collection konfigurieren",
    "Create Offline Collector: Configure artifact parameters":"Offline-Collector erstellen: Artefaktparameter konfigurieren",
    "Create Offline Collector: Review request":"Offline-Collector erstellen: Anfrage prüfen",
    "Create Offline Collector: Create collector":"Offline-Collector erstellen: Collector erstellen",

    "Create Offline collector:  Configure Collector":"Offline-Collector erstellen: Collector konfigurieren",
    "Target Operating System":"Zielbetriebssystem",
    "Password":"Passwort",
    "Report Template":"Reportvorlage",
    "No Report":"Kein Report",
    "Collection Type":"Collection typ",
    "Zip Archive":"Zip-Archiv",
    "Google Cloud Bucket":"Google Cloud Bucket",
    "AWS Bucket":"AWS Bucket",
    "SFTP Upload":"SFTP Upload",
    "Velociraptor Binary":"Velociraptor Binary",
    "Temp directory":"Temp-Verzeichnis",
    "Temp location":"Temp Ort",
    "Compression Level":"Komprimierungslevel",
    "Output format":"Ausgabeformat",
    "CSV and JSON":"CSV und JSON",
    "Output Prefix":"Ausgabe-Präfix",
    "Output filename prefix":"Dateinamen-Präfix ausgeben",

    "DeleteHuntDialog": <>
                    <p>You are about to permanently stop and delete all data from this hunt.</p>
                    <p>Are you sure you want to cancel this hunt and delete the collected data?</p>
                        </>,

    "Started":"Gestartet",
    "Expires":"Läuft ab",
    "Scheduled":"Geplant",
    "New Hunt":"Neue Hunt",
    "Run Hunt":"Hunt starten",
    "Stop Hunt":"Hunt beenden",
    "Delete Hunt":"Hunt löschen",
    "Copy Hunt":"Hunt kopieren",
    "No hunts exist in the system. You can start a new hunt by clicking the New Hunt button above.":"Auf dem System sind keine Hunts vorhanden. Sie können eine neue Hunt starten, indem Sie oben auf die Schaltfläche \"Neue Hunt\" klicken.",
    "Please select a hunt above":"Bitte wähle oben eine Hunt aus",
    "Clients":"Clients",
    "Notebook for Hunt": hunt_id=>"Notizbuch für Hunt " + hunt_id,
    "Hunt ID":"Hunt-ID",
    "Creation Time":"Erstellungszeit",
    "Expiry Time":"Ablaufzeit",
    "Total scheduled":"Gesamt geplant",
    "Finished clients":"Fertige Clients",
    "Full Download":"Vollständiger Download",
    "Summary Download":"Zusammenfassung Download",
    "Summary (CSV Only)":"Zusammenfassung (nur CSV)",
    "Summary (JSON Only)":"Zusammenfassung (nur JSON)",
    "name":"Name",
    "size":"Größe",
    "date":"Datum",
    "New Hunt - Configure Hunt":"Neue Hunt - Hunt konfigurieren",
    "Hunt description":"Beschreibung der Hunt",
    "Expiry":"Ablauf",
    "Include Condition":"Bedingung einschließen",
    "Run everywhere":"Überall starten",
    "Exclude Condition":"Bedingung ausschließen",
    "Configure Hunt":"Hunt konfigurieren",
    "Estimated affected clients":"Geschätzte betroffene Clients",
    "All Known Clients":"Alle bekannten Clients",
    "1 Day actives":"1 Tag aktiv",
    "1 Week actives":"1 Woche aktiv",
    "1 Month actives":"1 Monat aktiv",
    "Create Hunt: Select artifacts to collect":"Hunt erstellen: Artefakte zum Sammeln auswählen",
    "Create Hunt: Configure artifact parameters":"Hunt erstellen: Artefaktparameter konfigurieren",
    "Create Hunt: Specify resource limits":"Hunt erstellen: Ressourcenlimits angeben",
    "Create Hunt: Review request":"Hunt erstellen: Anfrage prüfen",
    "Create Hunt: Launch hunt":"Hunt erstellen: Hunt starten",

    "ClientId": "Client-ID",
    "StartedTime":"Startzeit",
    "TotalBytes":"Gesamtbytes",
    "TotalRows":"Gesamtzeilen",

    "client_time":"Client-Zeit",
    "level":"Level",
    "message":"Nachricht",

    "RecursiveVFSMessage": path=><>
    Sie sind dabei, alle Dateien in <b>{path}</b> rekursiv abzurufen.
    <br/><br/>
    Dadurch können große Datenmengen vom Client übertragen werden. Das Standard-Upload-Limit beträgt 1 GB, aber Sie können es im Bildschirm „Gesammelte Artefakte“ ändern.
    </>,

    "Textview":"Textansicht",
    "HexView":"HexView",
    "Refresh this directory (sync its listing with the client)":"Dieses Verzeichnis aktualisieren (Eintrag mit dem Client synchronisieren)",
    "Recursively refresh this directory (sync its listing with the client)":"Dieses Verzeichnis rekursiv aktualisieren (Eintrag mit dem Client synchronisieren)",
    "Recursively download this directory from the client":"Dieses Verzeichnis rekursiv vom Client herunterladen",
    "View Collection":"Collection anzeigen",
    "Size":"Größe",
    "Mode":"Modus",
    "mtime":"mtime",
    "atime":"atime",
    "ctime":"ctime",
    "btime":"btime",
    "Mtime":"Mtime",
    "Atime":"Atime",
    "Ctime":"Ctime",
    "Btime":"Btime",
    "Properties":"Eigenschaften",
    "No data available. Refresh directory from client by clicking above.":"Keine Daten verfügbar. Aktualisieren Sie das Verzeichnis vom Client, indem Sie oben klicken.",
    "Please select a file or a folder to see its details here.":"Bitte wählen Sie eine Datei oder einen Ordner aus, um die Details hier anzuzeigen.",
    "Currently refreshing from the client":"Aktuelle Aktualisierung vom Client",
    "Recursively download files":"Dateien rekursiv herunterladen",

    "Home":"Home",
    "Hunt Manager":"Hunt Manager",
    "View Artifacts":"Artefakte anzeigen",
    "Server Events":"Server-Ereignisse",
    "Server Artifacts":"Server Artifakte",
    "Host Information":"Host-Informationen",
    "Virtual Filesystem":"Virtuelles Dateisystem",
    "Client Events":"Client-Ereignisse",
    "This is a notebook for processing a hunt.":"Dies ist ein Notizbuch zur Abwicklung einer Hunt.",
    "ToolLocalDesc":
    <>
    Das Tool wird bei Bedarf vom Velociraptor-Server an die Clients
    ausgeliefert. Der Client speichert das Tool auf seiner eigenen
    Festplatte und vergleicht den Hash, wenn es das nächste Mal
    benötigt wird. Die Tools werden nur heruntergeladen, wenn sich ihr
    Hash geändert hat.
    </>,
    "ServedFromURL": (url)=>
    <>
    Die Clients rufen das Tool bei Bedarf direkt
    von <a href={api.href(url)}>{url}</a> ab. Wenn der Hashwert nicht mit dem
    erwarteten Hashwert übereinstimmt, weisen die Clients die Datei zurück.
    </>,
    "ServedFromGithub": (github_project, github_asset_regex)=>
    <>
    Die Tool-URL wird von GitHub als die neueste Version des
    Projekts <b>{github_project}</b>, die
    mit <b>{github_asset_regex}</b> übereinstimmt,
    aktualisiert.
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

    "Include Labels":"Labels einschließen",
    "Exclude Labels":"Labels ausschließen",
    "? for suggestions":"? für Vorschläge",
    "Served from URL":"Bereitgestellt von URL",
    "Placeholder Definition":"Platzhalterdefinition",
    "Materialize Hash":"Hash materialisieren",
    "Tool":"Tool",
    "Override Tool":"Tools überschreiben",
    "Select file":"Datei auswählen",
    "Click to upload file":"Klicken, um Datei hochzuladen",
    "Set Serve URL":"Server-URL festlegen",
    "Served Locally":"Lokal ausliefern",
    "Tool Hash Known":"Tool-Hash bekannt",
    "Re-Download File":"Datei erneut herunterladen",
    'Re-Collect from the client': "Nochmal vom Client einsammeln",
    'Collect from the client': 'Beim Client abholen',
    "Tool Name":"Toolname",
    "Upstream URL":"Upstream-URL",
    "Endpoint Filename":"Dateiname auf dem Client",
    "Hash":"Hash",
    "Serve Locally":"Lokal servieren",
    "Serve URL":"URL bereitstellen",
    "Fetch from Client": "Vom Client abrufen",
    "Last Collected": "Zuletzt gesammelt",
    "Offset": "Offset",
    "Show All": "Zeige alles",
    "Recent Hosts": "Letzte Hosts",
    "Download JSON": "JSON herunterladen",
    "Download CSV": "CSV-Datei herunterladen",
    "Transform Table": "Transformationstabelle",
    "Transformed": "Transformiert",

    "Select a notebook from the list above.":"Wählen Sie ein Notizbuch aus der obigen Liste aus.",
    "Cancel":"Abbrechen",
    "Recalculate":"Neu berechnen",
    "Stop Calculating":"Berechnung beenden",
    "Edit Cell":"Zelle bearbeiten",
    "Up Cell":"Zelle nach oben",
    "Down Cell":"Zelle unten",
    "Add Cell":"Zelle hinzufügen",
    "Suggestion":"Vorschlag",
    "Suggestions":"Vorschläge",
    "Add Timeline":"Zeitachse hinzufügen",
    "Add Cell From This Cell":"Zelle aus dieser Zelle hinzufügen",
    "Add Cell From Hunt":"Zelle von Hunt hinzufügen",
    "Add Cell From Flow":"Zelle aus Fluss hinzufügen",
    "Rendered":"Gerendert",
    "Undo":"Rückgängig machen",
    "Delete Cell":"Zelle löschen",
    "Uptime":"Verfügbarkeit",
    "BootTime":"BootTime",
    "Procs":"Procs",
    "OS":"Betriebssystem",
    "Platform":"Platform",
    "PlatformFamily":"Platformfamilie",
    "PlatformVersion":"Platformversion",
    "KernelVersion":"Kernelversion",
    "VirtualizationSystem":"Virtualizationsystem",
    "VirtualizationRole":"Virtualizationrolle",
    "HostID":"HostID",
    "Exe":"Exe",
    "Fqdn":"Fqdn",
    "Create a new Notebook":"Neues Notizbuch erstellen",
    "Collaborators":"Mitarbeiter",
    "Submit":"Absenden",
    "Edit notebook ":"Notizbuch bearbeiten ",
    "Notebook uploads":"Notebook-Uploads",
    "User Settings":"Benutzereinstellungen",
    "Select a user": "Wähle einen Benutzer",

    "Theme":"Theme",
    "Select a theme":"Wählen Sie ein Theme aus",
    "Default Velociraptor":"Standard-Velociraptor",
    "Velociraptor Classic (light)": "Velociraptor-Klassiker (hell)",
    "Velociraptor (light)":"Velociraptor (hell)",
    "Velociraptor (dark)":"Velociraptor (dunkel)",
    "Github dimmed (dark)":"Github gedimmt (dunkel)",
    "Github (light)": "Github (hell)",
    "Ncurses (dark)": "Ncurses (dunkel)",
    "Ncurses (light)":"Ncurses (hell)",
    "Cool Gray (dark)":"Cool Grey (dunkel)",
    "Strawberry Milkshake (light)":"Erdbeer-Milchshake (hell)",
    "Downloads Password":"Download-Passwort",
    "Default password to use for downloads":"Standardpasswort für Downloads",

    "Create Artifact from VQL":"Artefakt aus VQL erstellen",
    "Member":"Mitglied",
    "Response":"Antwort",
    "Super Timeline":"Super Timeline",
    "Super-timeline name":"Name der Super Timeline",
    "Timeline Name":"Name der Timeline",
    "Child timeline name":"Name der untergeordneten Timeline",
    "Time column":"Zeit Spalte",
    "Time Column":"Zeit Spalte",
    "Language": "Sprache",
    "Match by label": "Übereinstimmung nach Label",
    "All known Clients": "Alle bekannten Clients",
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
    "Transform table": "Transformationstabelle",
    "Sort Column": "Spalte sortieren",
    "Filter Regex": "Regex filtern",
    "Filter Column": "Spalte filtern",
    "Select label to edit its event monitoring table": "Label auswählen, um seine Event-Monitoringtabelle zu bearbeiten",
    "EventMonitoringCard":
    <>
    Die Event-Monitoring zielt auf bestimmte Labelgruppen ab. Wählen Sie oben
    eine Kennzeichnungsgruppe aus, um spezifische Ereignisartefakte für
    diese Gruppe zu konfigurieren.
    </>,
    "Event Monitoring: Configure Label groups": "Event-Monitoring: Labelgruppen konfigurieren",
    "Configuring Label": "Label konfigurieren",
    "Event Monitoring Label Groups": "Labelgruppen für das Event-Monitoring",
    "Event Monitoring: Select artifacts to collect from label group ": "Event-Monitoring: Zu sammelnde Artefakte aus der Labelgruppe auswählen ",
    "Artifact Collected": "Artefakt gesammelt",
    "Event Monitoring: Configure artifact parameters for label group ": "Event-Monitoring: Konfigurieren Sie Artefaktparameter für die Labelgruppe ",
    "Event Monitoring: Review new event tables": "Event-Monitoring: Neue Eventtabellen überprüfen",

    "Server Event Monitoring: Select artifacts to collect on the server":"Server Event-Monitoring: Wählen Sie Artefakte aus, die auf dem Server gesammelt werden sollen",
    "Server Event Monitoring: Configure artifact parameters for server":"Server Event-Monitoring: Artefaktparameter für Server konfigurieren",
    "Server Event Monitoring: Review new event tables":"Server Event-Monitoring: Neue Eventtabellen überprüfen",
    "Configure Label Group":"Labelgruppe konfigurieren",
    "Select artifact": "Artefakt auswählen",

    "Raw Data":"Rohdaten",
    "Logs":"Logdatei",
    "Log":"Logdatei",
    "Report":"Report",

    "NotebookId":"Notizbuch-ID",
    "Modified Time":"Geänderte Zeit",
    "Time": "Zeit",
    "No events": "Keine Ereignisse",
    "_ts": "Serverzeit",

    "Timestamp":"Zeitstempel",
    "started":"Gestartet",
    "vfs_path":"VFS Pfad",
    "file_size":"Dateigröße",
    "uploaded_size":"Hochgeladene Größe",
    "Select a language":"Sprache auswählen",
    "English":"Englisch",
    "Deutsch":"Deutsch",
    "Spanish": "Spanisch",
    "Portuguese": "Portugiesisch",
    "French": "Französisch",
    "Japanese": "Japanisch",

    "Type":"Typ",
    "Export notebooks":"Notizbücher exportieren",
    "Export to HTML":"Nach HTML exportieren",
    "Export to Zip":"Nach Zip exportieren",

    "Permanently delete Notebook":"Notizbuch endgültig löschen",
    "You are about to permanently delete the notebook for this hunt":"Sie sind dabei, das Notizbuch für diese Hunt endgültig zu löschen",

    "Data":"Daten",
    "Served from GitHub":"Von GitHub bereitgestellt",
    "Refresh Github":"Von GitHub aktualisieren",
    "Github Project":"GitHub-Projekt",
    "Github Asset Regex":"Github Asset Regex",
    "Admin Override":"Admin-Überschreibung",
    "Serve from upstream":"Vom Upstream servieren",

    "Update server monitoring table":"Server-Überwachungstabelle aktualisieren",
    "Show server monitoring tables":"Server-Überwachungstabellen anzeigen",

    "Display timezone": "Zeitzone anzeigen",
    "Select a timezone": "Wählen Sie eine Zeitzone aus",

    "Update client monitoring table":"Client-Überwachungstabelle aktualisieren",
    "Show client monitoring tables":"Client-Überwachungstabellen anzeigen",
    "Update Password": "Passwort aktualisieren",
    "Retype Password": "Passwort erneut eingeben",
    "Passwords do not match": "Passwörter stimmen nicht überein",

    "Delete Events": "Ereignisse löschen",
    "Urgent": "Dringend",
    "Skip queues and run query urgently": "Warteschlangen überspringen und Abfrage dringend ausführen",

    // Below need verification
    "Role_administrator": "Server-Administrator",
     "Role_org_admin": "Organisationsadministrator",
     "Role_reader": "Nur-Lese-Benutzer",
     "Role_analyst": "Analyst",
     "Role_investigator": "Ermittler",
     "Role_artifact_writer": "Artefaktschreiber",
     "Role_api": "Schreibgeschützter API-Client",

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


     "ToolPerm_ANY_QUERY": "Jede Abfrage überhaupt ausgeben",
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
    Deutsch[hex2a(k)] = v;
});

export default Deutsch;
