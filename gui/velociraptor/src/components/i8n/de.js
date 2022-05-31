import Alert from 'react-bootstrap/Alert';
import humanizeDuration from "humanize-duration";

const Deutsch = {
    SEARCH_CLIENTS: "Clients Suchen",
    "Quarantine description": (
        <>
          <p>Sie sind dabei, diesen Host unter Quarantäne zu stellen.</p>
          <p>
            Während der Quarantäne kann der Host dies nicht
            kommunizieren mit allen anderen Netzwerken, außer dem
            Velociraptor-Server.
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
    "Agent Name": "Agentenname",
    "First Seen At": "Zuerst gesehen bei",
    "Last Seen At": "Zuletzt gesehen bei",
    "Last Seen IP": "Zuletzt gesehene IP",
    "Labels": "Etiketten",
    "Operating System": "Betriebssystem",
    "Hostname": "Hostname",
    "FQDN": "FQDN",
    "Release": "Veröffentlichung",
    "Architecture": "Die Architektur",
    "Client Metadata": "Client-Metadaten",
    "Interrogate": "Abfragen",
    "VFS": "Virtuelles Dateisystem",
    "Collected": "Gesammelt",
    "Unquarantine Host": "Quarantäne-Host aufheben",
    "Quarantine Host": "Quarantäne-Host",
    "Quarantine Message": "Quarantänenachricht",
    "Add Label": "Etikett hinzufügen",
    "Overview": "Überblick",
    "VQL Drilldown": "Aufreißen",
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
    "Label Clients": "Client Etikett",
    "Existing": "Vorhandenen",
    "A new label": "Ein neues Etikett",
    "Add it!": "Fügen Sie es hinzu!",
    "Delete Clients": "Clients löschen",
    "DeleteMessage": "Sie sind im Begriff, die folgenden Clients endgültig zu löschen",
    "Yeah do it!": "Ja, mach es!",
    "Goto Page": "Gehe zu Seite",
    "Table is Empty": "Die Tabelle ist leer",
    "OS Version": "Version des Betriebssystems",
    "Select a label": "Wählen Sie ein Etikett aus",
    "Expand": "Erweitern",
    "Collapse": "Zusammenbruch",
    "Hide Output": "Leistung ausblenden",
    "Load Output": "Leistung laden",
    "Stop": "Aufhören",
    "Delete": "Löschen",
    "Run command on client": "Befehl auf dem Client ausführen",
    "Type VQL to run on the client": "Geben Sie VQL ein, das auf dem Client ausgeführt werden soll",
    "Run VQL on client": "Führen Sie VQL auf dem Client aus",
    "Artifact details": "Artefaktdetails",
    "Artifact Name": "Artefaktname",
    "Upload artifacts from a Zip pack": "Laden Sie Artefakte aus einem Zip-Paket hoch",
    "Select artifact pack (Zip file with YAML definitions)": "Artefaktpaket auswählen (Zip-Datei mit YAML-Definitionen)",
    "Click to upload artifact pack file": "Klicken Sie hier, um die Artefaktpaketdatei hochzuladen",
    "Delete an artifact": "Löschen Sie ein Artefakt",
    "You are about to delete": name=>"Sie sind im Begriff zu löschen " + name,
    "Add an Artifact": "Fügen Sie ein Artefakt hinzu",
    "Edit an Artifact": "Bearbeiten Sie ein Artefakt",
    "Delete Artifact": "Artefakt löschen",
    "Hunt Artifact": "Jagdartefakt",
    "Collect Artifact": "Sammle Artefakt",
    "Upload Artifact Pack": "Artefaktpaket hochladen",
    "Search for artifact": "Artefakt suchen",
    "Search for an artifact to view it": "Suchen Sie nach einem Artefakt, um es anzuzeigen",
    "Edit Artifact": name=>{
        return "Artefakt bearbeiten " + name;
    },
    "Create a new artifact": "Erstellen Sie ein neues Artefakt",
    "Save": "Speichern",

    // Keyboard navigation.
    "Global hotkeys": "Globale Hotkeys",
    "Goto dashboard": "Gehe zum Dashboard",
    "Collected artifacts": "Gesammelte Artefakte",
    "Show/Hide keyboard hotkeys help": "Hilfe zum Ein-/Ausblenden von Tastatur-Hotkeys",
    "Focus client search box": "Fokus-Client-Suchfeld",
    "New Artifact Collection Wizard": "Neuer Assistent zum Sammeln von Artefakten",
    "Artifact Selection Step": "Artefaktauswahlschritt",
    "Parameters configuration Step": "Parameterkonfigurationsschritt",
    "Collection resource specification": "Spezifikation der Erfassungsressourcen",
    "Launch artifact": "Artefakt starten",
    "Go to next step": "Gehen Sie zum nächsten Schritt",
    "Go to previous step": "Gehen Sie zum vorherigen Schritt",
    "Select next collection": "Wählen Sie die nächste Sammlung aus",
    "Select previous collection": "Vorherige Sammlung auswählen",
    "View selected collection results": "Zeigen Sie ausgewählte Sammlungsergebnisse an",
    "View selected collection overview": "Ausgewählte Sammlungsübersicht anzeigen",
    "View selected collection logs": "Zeigen Sie ausgewählte Sammlungsprotokolle an",
    "View selected collection uploaded files": "Zeigen Sie ausgewählte hochgeladene Sammlungsdateien an",
    "Editor shortcuts": "Editor-Shortcuts",
    "Popup the editor configuration dialog": "Öffnen Sie den Konfigurationsdialog des Editors",
    "Save editor contents": "Editorinhalte speichern",
    "Keyboard shortcuts": "Tastatürkürzel",
    "Yes do it!": "Ja mach das!",
    "Inspect Raw JSON": "Rohes JSON prüfen",
    "Raw Response JSON": "Unformatierte Antwort JSON",
    "Show/Hide Columns": "Spalten ein-/ausblenden",
    "Set All": "Setzen Sie alle",
    "Clear All": "Alles löschen",
    "Exit Fullscreen": "Vollbild beenden",
    "Artifact Collection": "Sammlung von Artefakten",
    "Uploaded Files": "Hochgeladene Dateien",
    "Results": "Ergebnisse",
    "Flow Details": "Flow-Details",
    "Notebook for Collection": name=>"Notizbuch für die Sammlung "+name,
    "Please click a collection in the above table":"Bitte klicken Sie auf eine Sammlung in der obigen Tabelle",
    "Artifact Names": "Artefaktnamen",
    "Creator": "Schöpfer",
    "Create Time": "Zeit schaffen",
    "Start Time": "Startzeit",
    "Last Active": "Letzte Aktivität",
    "Duration": "Dauer",
    " Running...": " Betrieb...",
    "State": "Zustand",
    "Error": "Fehler",
    "CPU Limit": "CPU-Beschränkungen",
    "IOPS Limit": "IOPS-Beschränkungen",
    "Timeout": "Auszeit",
    "Max Rows": "Max. Zeilen",
    "Max MB": "Max MB",
    "Artifacts with Results": "Artefakte mit Ergebnissen",
    "Total Rows": "Gesamtzeilen",
    "Uploaded Bytes": "Hochgeladene Bytes",
    "Files uploaded": "Hochgeladene Dateien",
    "Download Results": "Ergebnisse herunterladen",
    "Set a password in user preferences to lock the download file.": "Legen Sie in den Benutzereinstellungen ein Passwort fest, um die Download-Datei zu sperren.",
    "Prepare Download": "Download vorbereiten",
    "Prepare Collection Report": "Sammlungsbericht erstellen",
    "Available Downloads": "Verfügbare Downloads",
    "Size (Mb)": "Größe (MB)",
    "Date": "Datum",
    "Unlimited": "Unbegrenzt",
    "rows": "Reihen",
    "Request sent to client": "Anfrage an Client gesendet",
    "Description": "Beschreibung",
    "Created": "Erstellt",
    "Manually add collection to hunt": "Sammlung manuell zur Jagd hinzufügen",
    "No compatible hunts.": "Keine kompatiblen Jagden.",
    "Please create a hunt that collects one or more of the following artifacts.":"Bitte erstellen Sie eine Jagd, die eines oder mehrere der folgenden Artefakte sammelt.",
    "Requests": "Anfragen",
    "Notebook": "Notizbuch",
    "Permanently delete collection": "Sammlung dauerhaft löschen",
    "ArtifactDeletionDialog": (session_id, artifacts, total_bytes, total_rows)=>
    <>
      Sie sind im Begriff, die Artefaktsammlung dauerhaft zu löschen
      <b>{session_id}</b>.
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
    "New Favorite name": "Neuer Lieblingsname",
    "Describe this favorite": "Beschreibe diesen Favoriten",
    "New Collection": "Neue Kollektion",
    "Add to hunt": "Zur Jagd hinzufügen",
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
    "RUNNING": "LAUFEND",
    "STOPPED": "GESTOPPT",
    "PAUSED": "ANGEHALTEN",
    "ERROR": "Fehler",
    "CANCELLED": "Abgesagt",
    "Search for artifacts...": "Suche nach Artefakten...",
    "Favorite Name": "Lieblingsname",
    "Artifact": "Artefakt",
    "No artifacts configured. Please add some artifacts to collect": "Keine Artefakte konfiguriert. Bitte fügen Sie einige Artefakte zum Sammeln hinzu",

    "Artifacts": "Artefakte",
    "Collected Artifacts": "Gesammelte Artefakte",
    "Flow ID": "Fluss-ID",
    "FlowId": "Fluss-ID",
    "Goto notebooks": "Gehe zu Notizbüchern",
    "Max Mb": "Max Mb",
    "Mb": "Mb",
    "Name": "Name",
    "Ops/Sec": "Operationen/Sek",
    "Rows": "Zeilen",
    "New Collection: Select Artifacts to collect":"Neue Sammlung: Artefakte zum Sammeln auswählen",
    "Select Artifacts":"Artefakte auswählen",
    "Configure Parameters":"Parameter konfigurieren",
    "Specify Resources":"Ressourcen angeben",
    "Review":"Rezension",
    "Launch":"Starten",
    "New Collection: Configure Parameters":"Neue Sammlung: Parameter konfigurieren",
    "New Collection: Specify Resources":"Neue Sammlung: Ressourcen angeben",
    "New Collection: Review request":"Neue Sammlung: Überprüfungsanfrage",
    "New Collection: Launch collection":"Neue Kollektion: Kollektion starten",

    "CPU Limit Percent":"CPU Limit Percent",
    "IOps/Sec":"IOps/Sec",
    "Max Execution Time in Seconds":"Max. Ausführungszeit in Sekunden",
    "Max Idle Time in Seconds":"Maximale Leerlaufzeit in Sekunden",
    "If set collection will be terminated after this many seconds with no progress.":"Wenn die Satzsammlung nach so vielen Sekunden ohne Fortschritt beendet wird.",
    "Max Mb Uploaded":"Max. MB hochgeladen",
    "Collection did not upload files":"Sammlung hat keine Dateien hochgeladen",

    "Create Offline collector: Select artifacts to collect":"Offline-Collector erstellen: Zu sammelnde Artefakte auswählen",
    "Configure Collection":"Sammlung konfigurieren",
    "Create Offline Collector: Configure artifact parameters":"Offline-Collector erstellen: Artefaktparameter konfigurieren",
    "Create Offline Collector: Review request":"Offline-Collector erstellen: Anfrage prüfen",
    "Create Offline Collector: Create collector":"Offline-Collector erstellen: Collector erstellen",

    "Create Offline collector:  Configure Collector":"Offline-Collector erstellen: Collector konfigurieren",
    "Target Operating System":"Zielbetriebssystem",
    "Password":"Passwort",
    "Report Template":"Berichtsvorlage",
    "No Report":"Kein Bericht",
    "Collection Type":"Sammlungstyp",
    "Zip Archive":"Zip-Archiv",
    "Google Cloud Bucket":"Google Cloud Bucket",
    "AWS Bucket":"AWS Bucket",
    "SFTP Upload":"SFTP Upload",
    "Velociraptor Binary":"Velociraptor Binär",
    "Temp directory":"Temp-Verzeichnis",
    "Temp location":"Temporärer Standort",
    "Compression Level":"Komprimierungsstufe",
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
    "New Hunt":"Neue Jagd",
    "Run Hunt":"Run Hunt",
    "Stop Hunt":"Stop Hunt",
    "Delete Hunt":"Jagd löschen",
    "Copy Hunt":"Copy Hunt",
    "No hunts exist in the system. You can start a new hunt by clicking the New Hunt button above.":"Im System sind keine Jagden vorhanden. Sie können eine neue Jagd starten, indem Sie oben auf die Schaltfläche \"Neue Jagd\" klicken.",
    "Please select a hunt above":"Bitte wähle oben eine Jagd aus",
    "Clients":"Clients",
    "Notebook for Hunt": hunt_id=>"Notizbuch für Jagd " + hunt_id,

    "Hunt ID":"Jagd-ID",
    "Creation Time":"Erstellungszeit",
    "Expiry Time":"Ablaufzeit",
    "Total scheduled":"Gesamt geplant",
    "Finished clients":"Fertige Kunden",
    "Full Download":"Vollständiger Download",
    "Summary Download":"Zusammenfassung Download",
    "Summary (CSV Only)":"Zusammenfassung (nur CSV)",
    "Summary (JSON Only)":"Zusammenfassung (nur JSON)",
    "name":"Name",
    "size":"Größe",
    "date":"Datum",
    "New Hunt - Configure Hunt":"Neue Jagd - Jagd konfigurieren",
    "Hunt description":"Jagdbeschreibung",
    "Expiry":"Ablauf",
    "Include Condition":"Bedingung einschließen",
    "Run everywhere":"Überall laufen",
    "Exclude Condition":"Bedingung ausschließen",
    "Configure Hunt":"Jagd konfigurieren",
    "Estimated affected clients":"Geschätzte betroffene Kunden",
    "All Known Clients":"Alle bekannten Clients",
    "1 Day actives":"1 Tag aktiv",
    "1 Week actives":"1 Woche aktiv",
    "1 Month actives":"1 Monat aktiv",
    "Create Hunt: Select artifacts to collect":"Jagd erstellen: Artefakte zum Sammeln auswählen",
    "Create Hunt: Configure artifact parameters":"Jagd erstellen: Artefaktparameter konfigurieren",
    "Create Hunt: Specify resource limits":"Jagd erstellen: Ressourcenlimits angeben",
    "Create Hunt: Review request":"Jagd erstellen: Anfrage prüfen",
    "Create Hunt: Launch hunt":"Jagd erstellen: Jagd starten",

    "ClientId": "Kunden-ID",
    "StartedTime":"Startzeit",
    "TotalBytes":"Gesamtbytes",
    "TotalRows":"Gesamtzeilen",

    "client_time":"Client-Zeit",
    "level":"Ebene",
    "message":"Nachricht",

    "RecursiveVFSMessage": path=><>
    Sie sind dabei, alle Dateien in <b>{path}</b> rekursiv abzurufen.
    <br/><br/>
    Dadurch können große Datenmengen vom Endpunkt übertragen werden. Das Standard-Upload-Limit beträgt 1 GB, aber Sie können es im Bildschirm „Gesammelte Artefakte“ ändern.
    </>,

    "Textview":"Textansicht",
    "HexView":"HexView",
    "Refresh this directory (sync its listing with the client)":"Dieses Verzeichnis aktualisieren (seinen Eintrag mit dem Client synchronisieren)",
    "Recursively refresh this directory (sync its listing with the client)":"Dieses Verzeichnis rekursiv aktualisieren (seinen Eintrag mit dem Client synchronisieren)",
    "Recursively download this directory from the client":"Dieses Verzeichnis rekursiv vom Client herunterladen",
    "View Collection":"Sammlung anzeigen",
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

    "Home":"Zu Hause",
    "Hunt Manager":"Jagd Manager",
    "View Artifacts":"Artefakte anzeigen",
    "Server Events":"Server-Ereignisse",
    "Server Artifacts":"Server Artifacts",
    "Host Information":"Host-Informationen",
    "Virtual Filesystem":"Virtuelles Dateisystem",
    "Client Events":"Client-Ereignisse",
    "This is a notebook for processing a hunt.":"Dies ist ein Notizbuch zur Abwicklung einer Jagd.",
    "ToolLocalDesc":
    <>
    Das Tool wird vom Velociraptor-Server bereitgestellt
    Kunden ggf. Der Kunde wird
    Zwischenspeichern Sie das Tool auf seiner eigenen Festplatte und vergleichen Sie als Nächstes den Hash
    Zeit es benötigt wird. Tools werden nur heruntergeladen, wenn ihre
    Haschisch hat sich geändert.
    </>,
    "ServedFromURL": (base_path, url)=>
    <>
    Kunden holen sich das Tool direkt von
    <a href={base_path + url}>{url}</a> wenn
    erforderlich. Beachten Sie, dass, wenn der Hash nicht mit dem übereinstimmt
    erwarteten Hash werden die Clients die Datei ablehnen.
    </>,
    "ServedFromGithub": (github_project, github_asset_regex)=>
    <>
    Tool-URL wird aktualisiert von
    GitHub als neueste Version des Projekts
    <b>{github_project}</b> das passt
    <b>{github_asset_regex}</b>
    </>,
    "PlaceHolder":
    <>
    Tool-Hash ist derzeit unbekannt. Das erste Mal das Werkzeug
    erforderlich ist, wird Velociraptor es von dort herunterladen
    Upstream-URL und berechnen Sie ihren Hash.
    </>,
    "ToolHash":
    <>
    Tool-Hash wurde berechnet. Wenn Kunden verwenden müssen
    Mit diesem Tool stellen sie sicher, dass dieser Hash mit dem übereinstimmt, was sie tun
    Download.
    </>,
    "AdminOverride":
    <>
    Tool wurde manuell von einem hochgeladen
    admin - es wird nicht automatisch auf dem aktualisiert
    nächstes Update des Velociraptor-Servers.
    </>,
    "ToolError":
    <>
    Der Hash des Tools ist nicht bekannt und keine URL
    ist definiert. Es wird unmöglich sein, dieses Tool in einem zu verwenden
    Artefakt, weil Velociraptor es nicht auflösen kann. Du
    kann eine Datei manuell hochladen.
    </>,
    "OverrideToolDesc":
    <>
    Als Administrator können Sie manuell eine Datei hochladen
    binär als dieses Werkzeug verwendet werden. Dies überschreibt die
    Upstream-URL-Einstellung und stellen Sie Ihr Tool allen zur Verfügung
    Artefakte, die es brauchen. Legen Sie alternativ eine URL für Clients fest
    Werkzeug zu holen.
    </>,

    "Include Labels":"Etiketten einschließen",
    "Exclude Labels":"Etiketten ausschließen",
    "? for suggestions":"? für Vorschläge",
    "Served from URL":"Bereitgestellt von URL",
    "Placeholder Definition":"Platzhalterdefinition",
    "Materialize Hash":"Hash materialisieren",
    "Tool":"Werkzeug",
    "Override Tool":"Werkzeug überschreiben",
    "Select file":"Datei auswählen",
    "Click to upload file":"Klicken, um Datei hochzuladen",
    "Set Serve URL":"Server-URL festlegen",
    "Served Locally":"Lokal servieren",
    "Tool Hash Known":"Tool-Hash bekannt",
    "Re-Download File":"Datei erneut herunterladen",
    'Re-Collect from the client': "Wiedereinholung vom Kunden",
    'Collect from the client': 'Beim Kunden abholen',
    "Tool Name":"Werkzeugname",
    "Upstream URL":"Upstream-URL",
    "Enpoint Filename":"Enpoint-Dateiname",
    "Hash":"Hash",
    "Serve Locally":"Lokal servieren",
    "Serve URL":"URL bereitstellen",
    "Fetch from Client": "Vom Client abrufen",
    "Last Collected": "Zuletzt gesammelt",
    "Offset": "Versatz",
    "Show All": "Zeige alles",
    "Recent Hosts": "Letzte Gastgeber",
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
    "Add Cell From Hunt":"Zelle von Jagd hinzufügen",
    "Add Cell From Flow":"Zelle aus Fluss hinzufügen",
    "Rendered":"Gerendert",
    "Undo":"Rückgängig machen",
    "Delete Cell":"Zelle löschen",
    "Uptime":"Verfügbarkeit",
    "BootTime":"BootTime",
    "Procs":"Procs",
    "OS":"Betriebssystem",
    "Platform":"Platform",
    "PlatformFamily":"PlatformFamily",
    "PlatformVersion":"PlatformVersion",
    "KernelVersion":"KernelVersion",
    "VirtualizationSystem":"VirtualizationSystem",
    "VirtualizationRole":"VirtualizationRole",
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

    "Theme":"Thema",
    "Select a theme":"Wählen Sie ein Thema aus",
    "Default Velociraptor":"Standard-Velociraptor",
    "Velociraptor (light)":"Velociraptor (hell)",
    "Velociraptor (dark)":"Velociraptor (dunkel)",
    "Github dimmed (dark)":"Github gedimmt (dunkel)",
    "Cool Gray (dark)":"Cool Grey (dunkel)",
    "Strawberry Milkshake (light)":"Erdbeer-Milchshake (hell)",
    "Downloads Password":"Download-Passwort",
    "Default password to use for downloads":"Standardpasswort für Downloads",

    "Create Artifact from VQL":"Artefakt aus VQL erstellen",
    "Member":"Mitglied",
    "Response":"Antwort",
    "Super Timeline":"Super Zeitleiste",
    "Super-timeline name":"Name der Super Zeitleiste",
    "Timeline Name":"Name der Zeitspalte",
    "Child timeline name":"Name der untergeordneten Zeitachse",
    "Time column":"Zeitspalte",
    "Time Column":"Zeitspalte",
    "Language": "Sprache",
    "Match by label": "Übereinstimmung nach Etikett",
    "All known Clients": "Alle bekannten Kunden",
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
    "Select label to edit its event monitoring table": "Label auswählen, um seine Ereignisüberwachungstabelle zu bearbeiten",
    "EventMonitoringCard":
    <>
    Die Ereignisüberwachung zielt auf bestimmte Labelgruppen ab.
    Wählen Sie oben eine Etikettengruppe aus, um sie spezifisch zu konfigurieren
    Ereignisartefakte, die auf diese Gruppe abzielen.
    </>,
    "Event Monitoring: Configure Label groups": "Ereignisüberwachung: Etikettengruppen konfigurieren",
    "Configuring Label": "Etikett konfigurieren",
    "Event Monitoring Label Groups": "Etikettengruppen für die Ereignisüberwachung",
    "Event Monitoring: Select artifacts to collect from label group ": "Ereignisüberwachung: Zu sammelnde Artefakte aus der Labelgruppe auswählen ",
    "Artifact Collected": "Artefakt gesammelt",
    "Event Monitoring: Configure artifact parameters for label group ": "Ereignisüberwachung: Konfigurieren Sie Artefaktparameter für die Labelgruppe ",
    "Event Monitoring: Review new event tables": "Ereignisüberwachung: Neue Ereignistabellen überprüfen",

    "Server Event Monitoring: Select artifacts to collect on the server":"Serverereignisüberwachung: Wählen Sie Artefakte aus, die auf dem Server gesammelt werden sollen",
    "Server Event Monitoring: Configure artifact parameters for server":"Serverereignisüberwachung: Artefaktparameter für Server konfigurieren",
    "Server Event Monitoring: Review new event tables":"Serverereignisüberwachung: Neue Ereignistabellen überprüfen",
    "Configure Label Group":"Etikettengruppe konfigurieren",
    "Select artifact": "Artefakt auswählen",

    "Raw Data":"Rohdaten",
    "Logs":"Logdatei",
    "Log":"Logdatei",
    "Report":"Bericht",

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
    "TablePagination": (from, to, size)=>
    <>Zeigt Zeile { from } bis { to } von { size }</>,

    "Select a language":"Sprache auswählen",
    "English":"Englisch",
    "Deutsch":"Deutsch",
    "Spanish": "Spanisch",
    "Portuguese": "Portugiesisch",

    "Type":"Typ",
    "Export notebooks":"Notizbücher exportieren",
    "Export to HTML":"Nach HTML exportieren",
    "Export to Zip":"Nach Zip exportieren",

    "Permanently delete Notebook":"Notizbuch dauerhaft löschen",
    "You are about to permanently delete the notebook for this hunt":"Sie sind dabei, das Notizbuch für diese Jagd dauerhaft zu löschen",

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
};

export default Deutsch;
