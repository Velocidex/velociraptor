import _ from 'lodash';
import hex2a from "./utils";
import React from 'react';
import Alert from 'react-bootstrap/Alert';
import humanizeDuration from "humanize-duration";
import api from '../core/api-service.jsx';

import automated from "./fr.json";

const French = {
    "SEARCH_CLIENTS": "Rechercher des clients",
    "Quarantine description": (
        <>
          <p>Vous êtes sur le point de mettre cet hôte en quarantaine.</p>
          <p>
            Pendant la quarantaine, l'hôte ne pourra pas
            communiquer avec tous les autres réseaux sauf avec le
            serveur Vélociraptor.
          </p>
        </>),
    "Cannot Quarantine host": "Impossible de mettre l'hôte en quarantaine",
    "Cannot Quarantine host message": (os_name, quarantine_artifact)=>
        <>
          <Alert variant="warning">
            { quarantine_artifact ?
              <p>Cette instance Velociraptor n'a pas l'artefact <b>{quarantine_artifact}</b> requis pour mettre en quarantaine les hôtes en cours d'exécution {os_name}.</p> :
              <p>Cette instance Velociraptor n'a pas de nom d'artefact défini pour mettre en quarantaine les hôtes en cours d'exécution {os_name}.</p>
            }
          </Alert>
        </>,
    "Client ID": "Identifiant client",
    "Agent Version": "Version de l'agent",
    "Agent Name": "Nom de l'agent",
    "First Seen At": "Vu pour la première fois à",
    "Last Seen At": "Vu pour la dernière fois à",
    "Last Seen IP": "Dernière IP vue",
    "Labels": "Libellés",
    "Operating System": "Système d'exploitation",
    "Hostname": "Nom d'hôte",
    "FQDN": "FQDN",
    "Release": "Version",
    "Architecture": "Architecture",
    "Client Metadata": "Métadonnées client",
    "Interrogate": "Requêtes",
    "VFS": "Système de fichiers virtuel",
    "Collected": "Collecté",
    "Unquarantine Host": "Hôte non mis en quarantaine",
    "Quarantine Host": "Hôte de quarantaine",
    "Quarantine Message": "Message de quarantaine",
    "Add Label": "Ajouter un libellé",
    "Overview": "Aperçu",
    "VQL Drilldown": "Développer VQL",
    "Shell": "Shell",
    "Close": "Fermer",
    "Connected": "Connecté",
    "seconds": "Secondes",
    "minutes": "Minutes",
    "hours": "Heures",
    "days": "Jours",
    "time_ago": function(value, unit) {
        unit = French[unit] || unit;
        return 'Avant ' + value + ' ' + unit;
    },
    "Online": "En ligne",
    "Label Clients": "Libellé client",
    "Existing": "Existant",
    "A new label": "Un nouveau label",
    "Add it!": "Ajoutez-le!",
    "Delete Clients": "Supprimer des clients",
    "Kill Clients": "Terminer des clients",
    "DeleteMessage": "Vous êtes sur le point de supprimer définitivement les clients suivants",
    "KillMessage": "Vous êtes sur le point de terminer les clients suivants",
    "Yeah do it!": "Oui, fais-le!",
    "Goto Page": "Aller à la page",
    "Table is Empty": "Le tableau est vide",
    "OS Version": "Version du système d'exploitation",
    "Select a label": "Sélectionner un libellé",
    "Expand": "Développer",
    "Collapse": "Panne",
    "Hide Output": "Masquer les performances",
    "Load Output": "Performances de charge",
    "Stop": "Arrêter",
    "Delete": "Supprimer",
    "Run command on client": "Exécuter la commande sur le client",
    "Type VQL to run on the client": "Entrez VQL à exécuter sur le client",
    "Run VQL on client": "Exécuter VQL sur le client",
    "Artifact details": "Détails de l'artefact",
    "Artifact Name": "Nom de l'artefact",
    "Upload artifacts from a Zip pack": "Téléverser des artefacts à partir d'un paquet zip",
    "Select artifact pack (Zip file with YAML definitions)": "Sélectionner le paquet d'artefact (fichier zip avec YAML)",
    "Click to upload artifact pack file": "Cliquez ici pour téléverser le fichier de paquet d'artefact",
    "Delete an artifact": "Supprimer un artefact",
    "You are about to delete": name=>"Vous êtes sur le point de supprimer " + name,
    "Add an Artifact": "Ajouter un artefact",
    "Edit an Artifact": "Modifier un artefact",
    "Delete Artifact": "Supprimer l'artefact",
    "Hunt Artifact": "Artefact de chasse",
    "Collect Artifact": "Récupérer l'artefact",
    "Upload Artifact Pack": "Téléverser le paquet d'artefact",
    "Search for artifact": "Trouver un artefact",
    "Search for an artifact to view it": "Recherchez un artefact pour le voir",
    "Edit Artifact": name=>{
        return "Modifier l'artefact" + name;
    },
    "Create a new artifact": "Créer un nouvel artefact",
    "Save": "Enregistrer",
    "Search": "Recherche",
    "Toggle Main Menu": "Basculer le menu principal",
    "Main Menu": "Menu principal",
    "Welcome": "Bienvenu",

    // Navigation au clavier.
    "Global hotkeys": "Raccourcis globaux",
    "Goto dashboard": "Aller au tableau de bord",
    "Collected artifacts": "Artefacts collectés",
    "Show/Hide keyboard hotkeys help": "Afficher/masquer l'aide des raccourcis clavier",
    "Focus client search box": "Champ de recherche client ciblé",
    "New Artifact Collection Wizard": "Assistant nouvelle collection d'artefacts",
    "Artifact Selection Step": "Étape de sélection d'artefact",
    "Parameters configuration Step": "Étape de configuration des paramètres",
    "Collection resource specification": "Spécification de ressource de capture",
    "Launch artifact": "Démarrer l'artefact",
    "Go to next step": "Aller à l'étape suivante",
    "Go to previous step": "Aller à l'étape précédente",
    "Select next collection": "Sélectionnez la collection suivante",
    "Select previous collection": "Sélectionner la collection précédente",
    "View selected collection results": "Afficher les résultats de la collection sélectionnée",
    "View selected collection overview": "Afficher l'aperçu de la collection sélectionnée",
    "View selected collection logs": "Afficher les journaux de collecte sélectionnés",
    "View selected collection uploaded files": "Afficher les fichiers de collection téléchargés sélectionnés",
    "Editor shortcuts": "Raccourcis de l'éditeur",
    "Popup the editor configuration dialog": "Ouvrir la boîte de dialogue de configuration du Bloc-notes",
    "Save editor contents": "Enregistrer le contenu de l'éditeur",
    "Keyboard shortcuts": "Raccourci clavier",
    "Yes do it!": "Oui, fais le!",
    "Inspect Raw JSON": "Vérifier JSON brut",
    "Raw Response JSON": "Réponse JSON non formatée",
    "Show/Hide Columns": "Afficher/Masquer les colonnes",
    "Set All": "Définir tout",
    "Clear All": "Tout supprimer",
    "Exit Fullscreen": "Quitter le plein écran",
    "Artifact Collection": "Collection d'artefacts",
    "Uploaded Files": "Fichiers téléversés",
    "Results": "Résultats",
    "Flow Details": "Détails du flux",
    "Notebook for Collection": name=>"Carnet de notes pour la collection "+name,
    "Please click a collection in the above table":"Veuillez cliquer sur une collection dans le tableau ci-dessus",
    "Artifact Names": "Noms d'artefacts",
    "Creator": "Créateur",
    "Create Time": "Gagner du temps",
    "Start Time": "Heure de début",
    "Last Active": "Activité récente",
    "Duration": "Durée",
    " Running...": " Fonctionnement...",
    "State": "Etat",
    "Error": "Erreur",
    "CPU Limit": "Limites du processeur",
    "IOPS Limit": "Limites IOPS",
    "Timeout": "Temps expiré",
    "Max Rows": "Nombre maximum de lignes",
    "Max MB": "Mo max",
    "Artifacts with Results": "Artefacts avec résultats",
    "Total Rows": "Nombre total de lignes",
    "Uploaded Bytes": "Octets téléversés",
    "Files uploaded": "Fichiers téléversés",
    "Download Results": "Télécharger les résultats",
    "Set a password in user preferences to lock the download file.": "Pour verrouiller le fichier par mot de passe.",
    "Prepare Download": "Préparer le téléchargement",
    "Prepare Collection Report": "Créer un rapport de synthèse",
    "Available Downloads": "Téléchargements disponibles",
    "Size (Mb)": "Taille (Mo)",
    "Date": "Date",
    "Unlimited": "Illimité",
    "rows": "Lignes",
    "Request sent to client": "Requête envoyée au client",
    "Description": "Description",
    "Created": "Créé",
    "Manually add collection to hunt": "Ajouter manuellement une collection à chasser",
    "No compatible hunts.": "Aucune chasse compatible.",
    "Please create a hunt that collects one or more of the following artifacts.":"Veuillez créer une chasse qui collecte un ou plusieurs des artefacts suivants.",
    "Requests": "Demandes",
    "Notebook":"Carnet de notes",
    "Permanently delete collection": "Supprimer définitivement la collection",
    "ArtifactDeletionDialog": (session_id, artifacts, total_bytes, total_rows)=>
    <>
       Vous êtes sur le point de supprimer définitivement la collection
       d'artefacts <b>{session_id}</b>.
       <br/>
       Cette collection avait les artefacts <b className="wrapped-text">
          {artifacts}
       </b>
       <br/><br/>

       Nous supposons de libérer { total_bytes.toFixed(0) } Mo de stockage
       de données et { total_rows } lignes.
     </>,
     "Save this collection to your Favorites": "Enregistrer cette collection dans vos favoris",
     "ArtifactFavorites": artifacts=>
     <>
       Vous pouvez facilement collecter la même collection de vos
       favoris à l'avenir.
       <br/>
       Cette collection était les artefacts <b>{artifacts}</b>
       <br/><br/>
     </>,
    "New Favorite name": "Nouveau nom de favori",
    "Describe this favorite": "Décrivez ce favori",
    "New Collection": "Nouvelle collection",
    "Add to hunt": "Ajouter à la chasse",
    "Delete Artifact Collection": "Supprimer la collection d'artefacts",
    "Cancel Artifact Collection": "Annuler la collecte d'artefacts",
    "Copy Collection": "Collection de copies",
    "Save Collection": "Enregistrer la collection",
    "Build offline collector": "Créer un collecteur hors ligne",
    "Notebooks": "Carnets de notes",
    "Full Screen": "Plein écran",
    "Delete Notebook": "Supprimer le carnet de notes",
    "Notebook Uploads": "Téléversement de carnet de notes",
    "Export Notebook": "Exporter le carnet de notes",
    "FINISHED": "TERMINÉ",
    "RUNNING": "EN COURS",
    "STOPPED": "ARRÊTÉ",
    "PAUSED": "ARRÊTÉ",
    "ERROR": "ERREUR",
    "INFO": "INFO",
    "DEBUG": "DÉBOGAGE",
    "CANCELLED": "ANNULÉ",
    "Search for artifacts...": "Recherche d'artefacts...",
    "Favorite Name": "Nom préféré",
    "Artifact": "Artefact",
    "No artifacts configured. Please add some artifacts to collect": "Aucun artefact configuré. Veuillez ajouter quelques artefacts à collecter",

    "Artifacts": "Artefacts",
    "Collected Artifacts": "Artefacts collectés",
    "Flow ID": "ID de flux",
    "FlowId": "ID de flux",
    "Goto notebooks": "Aller aux carnets de notes",
    "Max Mb": "Max Mb",
    "Mb": "Mo",
    "Name": "Nom",
    "Ops/Sec": "Opérations/s",
    "Rows": "Lignes",
    "New Collection: Select Artifacts to collect": "Nouvelle collection: sélectionnez les artefacts à collecter",
    "Select Artifacts":"Sélectionner les artefacts",
    "Configure Parameters":"Configurer les paramètres",
    "Specify Resources":"Spécifier les ressources",
    "Review":"Revoir",
    "Launch":"Démarrer",
    "New Collection: Configure Parameters":"Nouvelle collection: configurer les paramètres",
    "New Collection: Specify Resources":"Nouvelle collection: spécifier les ressources",
    "New Collection: Review request":"Nouvelle collection: demande d'examen",
    "New Collection: Launch collection":"Nouvelle collection: démarrer la collection",

    "CPU Limit Percent":"Pourcentage limite CPU",
    "IOps/Sec":"IOps/s",
    "Max Execution Time in Seconds":"Temps d'exécution maximal en secondes",
    "Max Idle Time in Seconds":"Temps d'inactivité maximal en secondes",
    "If set collection will be terminated after this many seconds with no progress.":"Si l'ensemble de collecte sera terminé après ce nombre de secondes sans progression.",
    "Max bytes Uploaded":"Max Mo téléversé",
    "Collection did not upload files":"La collection n'a pas de fichiers téléversés",

    "Create Offline collector: Select artifacts to collect":"Créer un collecteur hors ligne: sélectionnez les artefacts à collecter",
    "Configure Collection":"Configurer la collecte",
    "Create Offline Collector: Configure artifact parameters":"Créer un collecteur hors ligne: configurer les paramètres d'artefact",
    "Create Offline Collector: Review request": "Créer un collecteur hors ligne: vérifier la demande",
    "Create Offline Collector: Create collector":"Créer un collecteur hors ligne: Créer un collecteur",

    "Create Offline collector:  Configure Collector":"Créer un collecteur hors ligne: configurer le collecteur",
    "Target Operating System":"Système d'exploitation cible",
    "Password":"Mot de passe",
    "Report Template":"Modèle de rapport",
    "No Report":"Aucun rapport",
    "Collection Type":"Type de collecte",
    "Zip Archive":"Archive ZIP",
    "Google Cloud Bucket":"Google Cloud Bucket",
    "AWS Bucket":"AWS Bucket",
    "SFTP Upload":"Téléversement SFTP",
    "Velociraptor Binary":"Binaire Vélociraptor",
    "Temp directory":"Répertoire temporaire",
    "Temp location":"Emplacement temporaire",
    "Compression Level":"Niveau de compression",
    "Output format":"Format de sortie",
    "CSV and JSON":"CSV et JSON",
    "Output Prefix":"Préfixe de sortie",
    "Output filename prefix":"Imprimer le préfixe du nom de fichier",

    "DeleteHuntDialog": <>
                    <p>Vous êtes sur le point d'arrêter définitivement et de supprimer toutes les données de cette chasse.</p>
                    <p>Êtes-vous sûr de vouloir annuler cette chasse et supprimer les données collectées?</p>
                        </>,

    "Started":"Démarré",
    "Expires":"Expire",
    "Scheduled":"Programmé",
    "New Hunt":"Nouvelle chasse",
    "Run Hunt":"Exécutez la chasse",
    "Stop Hunt":"Arrêtez la chasse",
    "Delete Hunt":"Supprimer la chasse",
    "Copy Hunt":"Copier la chasse",
    "No hunts exist in the system. You can start a new hunt by clicking the New Hunt button above.":"Aucune chasse n'existe dans le système. Vous pouvez commencer une nouvelle chasse en cliquant sur le bouton Nouvelle chasse ci-dessus.",
    "Please select a hunt above":"Veuillez sélectionner une chasse ci-dessus",
    "Clients":"Clients",
    "Notebook for Hunt": hunt_id=>"Carnet de notes de chasse" + hunt_id,

    "Hunt ID":"ID de chasse",
    "Creation Time":"Heure de création",
    "Expiry Time":"Délai d'expiration",
    "Total scheduled":"Total prévu",
    "Finished clients":"Clients finis",
    "Full Download":"Téléchargement complet",
    "Summary Download":"Télécharger le résumé",
    "Summary (CSV Only)":"Résumé (CSV uniquement)",
    "Summary (JSON Only)":"Résumé (JSON uniquement)",
    "name":"Nom",
    "size":"Taille",
    "date":"Date",
    "New Hunt - Configure Hunt":"Nouvelle chasse - Configurer la chasse",
    "Hunt description":"Description de la chasse",
    "Expiry":"Expiration",
    "Include Condition":"Inclure la condition",
    "Run everywhere":"Courir partout",
    "Exclude Condition":"Exclure la condition",
    "Configure Hunt":"Configurer la chasse",
    "Estimated affected clients":"Estimation des clients concernés",
    "All Known Clients":"Tous les clients connus",
    "1 Day actives":"1 jour actif",
    "1 Week actives":"1 semaine active",
    "1 Month actives":"1 mois actif",
    "Create Hunt: Select artifacts to collect":"Créer une chasse: sélectionnez les artefacts à rassembler",
    "Create Hunt: Configure artifact parameters":"Créer une chasse: configurer les paramètres d'artefact",
    "Create Hunt: Specify resource limits":"Créer une chasse: spécifiez les limites de ressources",
    "Create Hunt: Review request":"Créer une recherche: vérifier la demande",
    "Create Hunt: Launch hunt":"Créer une chasse: démarrer la chasse",

    "ClientId": "Identifiant client",
    "StartedTime":"Heure de début",
    "TotalBytes":"Nombre total d'octets",
    "TotalRows":"Nombre total de lignes",

    "client_time":"Temps client",
    "level":"Niveau",
    "message":"Message",

    "RecursiveVFSMessage": path=><>
    Vous êtes sur le point de récupérer de manière récursive tous les fichiers dans <b>{path}</b>.
    <br/><br/>
    Cela permet de transférer de grandes quantités de données depuis le terminal. La limite de téléchargement par défaut est de 1 Go, mais vous pouvez la modifier dans l'écran des artefacts collectés.
    </>,

    "Textview":"Vue texte",
    "HexView":"Vue Hexadécimale",
    "Refresh this directory (sync its listing with the client)": "Mettre à jour ce répertoire (synchroniser son entrée) avec le client",
    "Recursively refresh this directory (sync its listing with the client)":"Actualisez récursivement ce répertoire (synchronisez sa liste avec le client)",
    "Recursively download this directory from the client":"Télécharger ce répertoire récursivement depuis le client",
    "View Collection":"Voir la collection",
    "Size":"Taille",
    "Mode":"Mode",
    "mtime":"mtime",
    "atime":"atime",
    "ctime":"ctime",
    "btime":"btime",
    "Mtime":"Mtime",
    "Atime":"Atime",
    "Ctime":"Ctime",
    "Btime":"Btime",
    "Properties":"Propriétés",
    "No data available. Refresh directory from client by clicking above.":"Pas de données disponibles. Actualiser le répertoire du client en cliquant ci-dessus.",
    "Please select a file or a folder to see its details here.":"Veuillez sélectionner un fichier ou un dossier ici.",
    "Currently refreshing from the client":"Mise à jour actuelle du client",
    "Recursively download files":"Télécharger des fichiers de manière récursive",

    "Home":"Maison",
    "Hunt Manager":"Gestionnaire de chasse",
    "View Artifacts":"Afficher les artefacts",
    "Server Events":"Événements du serveur",
    "Server Artifacts":"Artefacts du serveur",
    "Host Information":"Informations sur l'hôte",
    "Virtual Filesystem":"Système de fichiers virtuel",
    "Client Events":"Événements clients",
    "This is a notebook for processing a hunt.":"C'est un carnet de notes pour gérer une chasse.",
    "ToolLocalDesc":
    <>
    L'outil sera servi du serveur Velociraptor aux clients si nécessaire.
    Le client mettra l'outil en cache sur son propre disque et comparera
    le hachage la prochaine fois qu'il en aura besoin.
    Les outils ne seront téléchargés que si leur hachage a changé.
    </>,
    "ServedFromURL": (url)=>
    <>
    Les clients iront chercher l'outil directement à partir
    de <a href={api.href(url)}>{url}</a> si
    nécessaire. Notez que si le hachage ne correspond pas au
    hachage attendu, les clients rejetteront le fichier.
    </>,
    "ServedFromGithub": (github_project, github_asset_regex)=>
    <>
    L'URL de l'outil est mise à jour par
    GitHub comme dernière version du
    projet <b>{github_project}</b> qui
    correspond <b>{github_asset_regex}</b>
    </>,
    "PlaceHolder":
    <>
    Le hachage de l'outil est actuellement inconnu.
    La première fois que l'outil est nécessaire,
    Velociraptor le télécharge à partir de son URL
    en amont et calcule son hachage.
    </>,
    "ToolHash":
    <>
    Le hachage de l'outil a été calculé.
    Lorsque les clients ont besoin d'utiliser cet outil,
    ils s'assurent que ce hachage correspond à ce qu'ils téléchargent.
    </>,
    "AdminOverride":
    <>
    L'outil a été téléchargé manuellement par un administrateur
    - il ne sera pas automatiquement mis à jour
    lors de la prochaine mise à jour du serveur Velociraptor
    </>,
    "ToolError":
    <>
    Le hachage de l'outil n'est pas connu et aucune URL
    n'est définie. Il sera impossible d'utiliser cet outil
    dans un artefact car Velociraptor est incapable de le résoudre.
    Vous pouvez télécharger manuellement un fichier.
    </>,
    "OverrideToolDesc":
    <>
    En tant qu'administrateur, vous pouvez télécharger manuellement un fichier binaire
    à utiliser comme outil. Cela remplacera le paramètre d'URL en amont et fournira
    votre outil à tous les artefacts qui en ont besoin. Alternative, définissez une URL
    pour que les clients récupèrent les outils.
    </>,

    "Include Labels":"Inclure les libellés",
    "Exclude Labels":"Exclure les libellés",
    "? for suggestions":"? pour des suggestions",
    "Served from URL":"Fourni par l'URL",
    "Placeholder Definition":"Définition d'espace réservé",
    "Materialize Hash":"Matérialiser le hachage",
    "Tool":"Outil",
    "Override Tool":"Outil d'écrasement",
    "Select file":"Sélectionner un fichier",
    "Click to upload file":"Cliquez pour télécharger le fichier",
    "Set Serve URL":"Définir l'URL du serveur",
    "Served Locally":"Servir localement",
    "Tool Hash Known":"Hachage d'outil connu",
    "Re-Download File":"Télécharger le fichier à nouveau",
    'Re-Collect from the client': "Récupérer auprès du client",
    'Collect from the client': 'Collecte auprès du client',
    "Tool Name":"Nom de l'outil",
    "Upstream URL":"URL en amont",
    "Endpoint Filename":"nom de fichier sur client",
    "Hash":"Hachage",
    "Serve Locally":"Servir localement",
    "Serve URL":"Fournir l'URL",
    "Fetch from Client": "Obtenir du client",
    "Last Collected": "Choisi en dernier",
    "Offset": "Décalage",
    "Show All": "Afficher tout",
    "Recent Hosts": "Hôtes récents",
    "Download JSON": "Télécharger JSON",
    "Download CSV": "Télécharger le fichier CSV",
    "Transform Table": "Tableau de transformation",
    "Transformed": "Transformé",

    "Select a notebook from the list above.":"Sélectionnez un carnet de notes dans la liste ci-dessus.",
    "Cancel":"Annuler",
    "Recalculate":"Recalculer",
    "Stop Calculating":"Fin du calcul",
    "Edit Cell":"Modifier la cellule",
    "Up Cell":"Cellule vers le haut",
    "Down Cell":"Cellule vers le bas",
    "Add Cell":"Ajouter une cellule",
    "Suggestion":"Suggestion",
    "Suggestions":"Suggestions",
    "Add Timeline":"Ajouter une chronologie",
    "Add Cell From This Cell":"Ajouter une cellule à partir de cette cellule",
    "Add Cell From Hunt":"Ajouter une cellule de Hunt",
    "Add Cell From Flow":"Ajouter une cellule à partir du flux",
    "Rendered":"Rendu",
    "Undo":"Annuler",
    "Delete Cell":"Supprimer la cellule",
    "Uptime":"Disponibilité",
    "BootTime":"Temps de démarrage",
    "Procs":"Traitement",
    "OS":"Système d'exploitation",
    "Platform":"Plate-forme",
    "PlatformFamily":"Famille de plate-forme",
    "PlatformVersion":"Version de plate-forme",
    "KernelVersion":"Version du noyau",
    "VirtualizationSystem":"Système de virtualisation",
    "VirtualizationRole":"Rôle de virtualisation",
    "HostID":"ID d'hôte",
    "Exe":"Exe",
    "Fqdn":"Fqdn",
    "Create a new Notebook":"Créer un nouveau carnet de notes",
    "Collaborators":"Employé",
    "Submit":"Soumettre",
    "Edit notebook ":"Modifier le carnet de notes",
    "Notebook uploads":"Téléversements de carnet de notes",
    "User Settings":"Paramètres utilisateur",
    "Select a user": "Sélectionner un utilisateur",

    "Theme":"Thème",
    "Select a theme":"Sélectionnez un thème",
    "Default Velociraptor":"Vélociraptor standard",
    "Velociraptor Classic (light)": "Vélociraptor Classique (léger)",
    "Velociraptor (light)":"Vélociraptor (léger)",
    "Velociraptor (dark)":"Vélociraptor (foncé)",
    "Github (light)": "Github (léger)",
    "Github dimmed (dark)":"Github estompé (foncé)",
    "Ncurses (dark)": "Ncurses (foncé)",
    "Ncurses (light)":"Ncurses (léger)",
    "Cool Gray (dark)":"Gris froid (foncé)",
    "Strawberry Milkshake (light)":"Milkshake aux fraises (léger)",
    "Midnight Inferno (very dark)": "Minuit Inferno (très foncé)",
    "Downloads Password":"Télécharger le mot de passe",
    "Default password to use for downloads":"Mot de passe par défaut pour les téléchargements",

    "Create Artifact from VQL":"Créer un artefact à partir de VQL",
    "Member":"Membre",
    "Response":"Réponse",
    "Super Timeline":"Grande chronologie",
    "Super-timeline name":"Nom de la super chronologie",
    "Timeline Name":"Nom de la chronologie",
    "Child timeline name":"Nom de la chronologie enfant",
    "Time column":"Colonne d'heure",
    "Time Column":"Colonne de temps",
    "Language": "Langue",
    "Match by label": "Correspondance par label",
    "All known Clients": "Tous les clients connus",
    "X per second": x=><>{x} par seconde</>,
    "HumanizeDuration": difference=>{
        if (difference<0) {
            return<>
                     Dans {humanizeDuration(difference, {
                         round: true,
                         langue: "fr",
                     })}
                   </>;
        }
        return<>
                 Avant {humanizeDuration(difference, {
                     round: true,
                     langue: "fr",
                 })} ago
               </>;
    },
    "Transform table": "Tableau de transformation",
    "Sort Column": "Trier la colonne",
    "Filter Regex": "Filtre Regex",
    "Filter Column": "Colonne de filtre",
    "Select label to edit its event monitoring table": "Sélectionnez le libellé pour modifier sa table de surveillance des événements",
    "EventMonitoringCard":
    <>
    La surveillance des événements cible des groupes de labels spécifiques.
    Sélectionnez un groupe de labels ci-dessus pour configurer des
    artefacts d'événement spécifiques ciblant ce groupe.
    </>,
    "Event Monitoring: Configure Label groups": "Moniteur d'événements: configurer les groupes de libellés",
    "Configuring Label": "Configurer le label",
    "Event Monitoring Label Groups": "Groupes de label de moniteur d'événements",
    "Event Monitoring: Select artifacts to collect from label group ": "Surveillance des événements: sélectionner les artefacts à collecter à partir du groupe de libellés ",
    "Artifact Collected": "Artefact collecté",
    "Event Monitoring: Configure artifact parameters for label group": "Configurer les paramètres de surveillance des événements",
    "Event Monitoring: Review new event tables": "Moniteur d'événements: vérifier les nouvelles tables d'événements",

    "Server Event Monitoring: Select artifacts to collect on the server":"Surveillance des événements du serveur: sélectionner les artefacts à collecter sur le serveur",
    "Server Event Monitoring: Configure artifact parameters for server":"Surveillance des événements du serveur: configurer les paramètres d'artefact pour le serveur",
    "Server Event Monitoring: Review new event tables":"Surveillance des événements du serveur: vérifier les nouvelles tables d'événements",
    "Configure Label Group":"Configurer le groupe de libellés",
    "Select artifact": "Sélectionner l'artefact",

    "Raw Data":"Données brutes",
    "Logs":"Journaux",
    "Log":"Journal",
    "Report":"Rapport",

    "NotebookId":"Identifiant du carnet de notes",
    "Modified Time":"Heure modifiée",
    "Time": "Heure",
    "No events": "Numéro des événements",
    "_ts": "Heure du serveur",

    "Timestamp":"Horodatage",
    "started":"Démarré",
    "vfs_path":"Chemin VFS",
    "file_size":"Taille du fichier",
    "uploaded_size":"Taille téléversée",

    "Select a language":"Sélectionner la langue",
    "English":"Anglais",
    "Deutsch":"Allemand",
    "Spanish":"Espagnol",
    "Portuguese":"Portugais",
    "French":"Français",
    "Japanese": "Japonais",

    "Type":"Type",
    "Export notebooks":"Exporter les carnets de notes",
    "Export to HTML":"Exporter au format HTML",
    "Export to Zip":"Exporter au format Zip",

    "Permanently delete Notebook":"Supprimer définitivement le carnet de notes",
    "You are about to permanently delete the notebook for this hunt":"Vous êtes sur le point de supprimer définitivement le carnet de notes de cette chasse",

    "Data":"Données",
    "Served from GitHub":"Fourni par GitHub",
    "Refresh Github":"Mise à jour depuis GitHub",
    "Github Project":"Projet GitHub",
    "Github Asset Regex":"Regex d'actif Github",
    "Admin Override":"Remplacement de l'administrateur",
    "Serve from upstream":"Servir depuis l'amont",

    "Update server monitoring table":"Mettre à jour le tableau de surveillance du serveur",
    "Show server monitoring tables":"Afficher les tableaux de surveillance du serveur",

    "Display timezone": "Afficher le fuseau horaire",
    "Select a timezone": "Sélectionner un fuseau horaire",

    "Update client monitoring table":"Mettre à jour le tableau de surveillance des clients",
    "Show client monitoring tables":"Afficher les tableaux de surveillance des clients",
    "Urgent": "Urgent",
    "Skip queues and run query urgently": "Ignorer les files d'attente et exécuter la requête de toute urgence",

    // Below need verification
    "Role_administrator": "Administrateur du serveur",
    "Role_org_admin": "Administrateur de l'organisation",
    "Role_reader": "Utilisateur en lecture seule",
    "Rôle_analyste": "Analyste",
    "Role_investigator": "Enquêteur",
    "Role_artifact_writer": "Auteur d'artefacts",
    "Role_api": "Client API en lecture seule",

     "Perm_ANY_QUERY": "N'importe quelle requête",
     "Perm_PUBISH": "Publier",
     "Perm_READ_RESULTS": "Lire les résultats",
     "Perm_LABEL_CLIENT": "Étiqueter les clients",
     "Perm_COLLECT_CLIENT": "Recueillir le client",
     "Perm_START_HUNT": "Démarrer la chasse",
     "Perm_COLLECT_SERVER": "Serveur de collecte",
     "Perm_ARTIFACT_WRITER": "Auteur d'artefacts",
     "Perm_SERVER_ARTIFACT_WRITER": "Auteur d'artefacts de serveur",
     "Perm_EXECVE": "EXECVE",
     "Perm_NOTEBOOK_EDITOR": "Éditeur de bloc-notes",
     "Perm_SERVER_ADMIN": "Administrateur du serveur",
     "Perm_ORG_ADMIN": "Administrateur de l'organisation",
     "Perm_IMPERSONATION": "Usurpation d'identité",
     "Perm_FILESYSTEM_READ": "Lecture du système de fichiers",
     "Perm_FILESYSTEM_WRITE": "Écriture du système de fichiers",
     "Perm_MACHINE_STATE": "État de la machine",
     "Perm_PREPARE_RESULTS": "Préparer les résultats",
     "Perm_DATASTORE_ACCESS": "Accès à la banque de données",

     "ToolPerm_ANY_QUERY": "Emettre n'importe quelle requête",
     "ToolPerm_PUBISH": "Publier les événements dans les files d'attente côté serveur (généralement non nécessaires)",
     "ToolPerm_READ_RESULTS": "Lire les résultats des chasses, des flux ou des cahiers déjà exécutés",
     "ToolPerm_LABEL_CLIENT": "Peut manipuler les étiquettes et les métadonnées des clients",
     "ToolPerm_COLLECT_CLIENT": "Planifier ou annuler de nouvelles collectes sur les clients",
     "ToolPerm_START_HUNT": "Démarrer une nouvelle chasse",
     "ToolPerm_COLLECT_SERVER": "Planifier de nouvelles collections d'artefacts sur les serveurs Velociraptor",
     "ToolPerm_ARTIFACT_WRITER": "Ajouter ou modifier des artefacts personnalisés qui s'exécutent sur le serveur",
     "ToolPerm_SERVER_ARTIFACT_WRITER": "Ajouter ou modifier des artefacts personnalisés qui s'exécutent sur le serveur",
     "ToolPerm_EXECVE": "Autorisé à exécuter des commandes arbitraires sur les clients",
     "ToolPerm_NOTEBOOK_EDITOR": "Autorisé à changer de cahiers et de cellules",
     "ToolPerm_SERVER_ADMIN": "Autorisé à gérer la configuration du serveur",
     "ToolPerm_ORG_ADMIN": "Autorisé à gérer les organisations",
     "ToolPerm_IMPERSONATION": "Permet à l'utilisateur de spécifier un nom d'utilisateur différent pour le plugin query()",
     "ToolPerm_FILESYSTEM_READ": "Autorisé à lire des fichiers arbitraires du système de fichiers",
     "ToolPerm_FILESYSTEM_WRITE": "Autorisé à créer des fichiers sur le système de fichiers",
     "ToolPerm_MACHINE_STATE": "Autorisé à collecter des informations sur l'état des machines (par exemple, pslist())",
     "ToolPerm_PREPARE_RESULTS": "Autorisé à créer des fichiers zip",
     "ToolPerm_DATASTORE_ACCESS": "Accès au magasin de données brut autorisé",
};

_.each(automated, (v, k)=>{
    French[hex2a(k)] = v;
});

export default French;
