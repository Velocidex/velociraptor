import _ from 'lodash';
import hex2a from "./utils";
import React from 'react';
import Alert from 'react-bootstrap/Alert';
import humanizeDuration from "humanize-duration";
import api from '../core/api-service.jsx';

import automated from "./es.json";

const Spanish = {
    "SEARCH_CLIENTS": "Buscar Clientes",
    "Quarantine description": (<>
          <p>Está a punto de poner en cuarentena este host.</p>
          <p>
            Durante la cuarentena, el host no podrá
            comunicarse con ninguna otra red, salvo con el
            servidor de Velociraptor.
          </p>
        </>),
    "Cannot Quarantine host": "No se puede poner en cuarentena el host",
    "Cannot Quarantine host message": (os_name, quarantine_artifact)=>
        <>
          <Alert variant="warning">
            { quarantine_artifact ?
              <p>Esta instancia de Velociraptor no tiene el artefacto <b>{quarantine_artifact}</b> necesario para poner en cuarentena los hosts que ejecutan {os_name}.</p> :
              <p>Esta instancia de Velociraptor no tiene un nombre de artefacto definido para poner en cuarentena los hosts que ejecutan {os_name}.</p>
            }
          </Alert>
        </>,
    "Client ID": "ID del cliente",
    "Agent Version": "Versión del agente",
    "Agent Name": "Nombre del Agente",
    "First Seen At": "Visto por primera vez en",
    "Last Seen At": "Visto por última vez en",
    "Last Seen IP": "Última IP vista",
    "Labels": "Etiquetas",
    "Operating System": "Sistema Operativo",
    "Hostname": "Nombre de host",
    "FQDN": "FQDN",
    "Release": "Liberar",
    "Architecture": "Arquitectura",
    "Client Metadata": "Metadatos del cliente",
    "Interrogate": "Interrogar",
    "VFS": "Sistema de archivos virtual",
    "Collected": "Recopilado",
    "Unquarantine Host": "Levantar cuarentena",
    "Quarantine Host": "Poner en cuarentena",
    "Quarantine Message": "Mensaje de cuarentena",
    "Add Label": "Agregar etiqueta",
    "Overview": "Resumen",
    "VQL Drilldown": "Desglose",
    "Shell": "Consola",
    "Close": "Cerrar",
    "Connected": "Conectado",
    "seconds": "segundos",
    "minutes": "minutos",
    "hours": "horas",
    "days": "días",
    "time_ago": function(value, unit) {
        unit = Spanish[unit] || unit;
        return 'Hace ' + value + ' ' + unit;
    },
    "Online": "En línea",
    "Label Clients": "Etiquetar clientes",
    "Existing": "Existente",
    "A new label": "Nueva etiqueta",
    "Add it!": "¡Añádelo!",
    "Delete Clients": "Eliminar Clientes",
    "DeleteMessage": "Está a punto de eliminar permanentemente los siguientes clientes",
    "Yeah do it!": "¡Sí, hazlo!",
    "Goto Page": "Ir a página",
    "Table is Empty": "Tabla vacía",
    "OS Version": "Versión del sistema operativo",
    "Select a label": "Seleccionar una etiqueta",
    "Expand": "Expandir",
    "Collapse": "Contraer",
    "Hide Output": "Ocultar salida",
    "Load Output": "Cargar salida",
    "Stop": "Parar",
    "Delete": "Eliminar",
    "Run command on client": "Ejecutar comando en cliente",
    "Type VQL to run on the client": "Escriba consulta VQL a ejecutar en el cliente",
    "Run VQL on client": "Ejecutar consulta VQL en el cliente",
    "Artifact details": "Detalles del artefacto",
    "Artifact Name": "Nombre del artefacto",
    "Upload artifacts from a Zip pack": "Cargar artefactos desde un fichero Zip",
    "Select artifact pack (Zip file with YAML definitions)": "Seleccionar paquete de artefactos (archivo zip con definiciones YAML)",
    "Click to upload artifact pack file": "Haga clic para cargar el archivo del paquete de artefacto",
    "Delete an artifact": "Eliminar un artefacto",
    "You are about to delete": name=>"Está a punto de eliminar  " + name,
    "Add an Artifact": "Agregar un artefacto",
    "Edit an Artifact": "Editar un artefacto",
    "Delete Artifact": "Eliminar artefacto",
    "Hunt Artifact": "Hunt sobre Artefacto",
    "Collect Artifact": "Recolectar Artefacto",
    "Upload Artifact Pack": "Cargar paquete de artefacto",
    "Search for artifact": "Buscar artefacto",
    "Search for an artifact to view it": "Buscar un artefacto para verlo",
    "Edit Artifact": name=>{
        return "Editar artefacto " + name;
    },
    "Create a new artifact": "Crear un nuevo artefacto",
    "Save": "Guardar",
    "Search": "Buscar",
    "Toggle Main Menu": "Alternar menú principal",
    "Main Menu": "Menú principal",
    "Welcome": "Bienvenido",

    // Keyboard navigation.
    "Global hotkeys": "Teclas de acceso directo globales",
    "Goto dashboard": "Ir al dashboard",
    "Collected artifacts": "Artefactos recopilados",
    "Show/Hide keyboard hotkeys help": "Mostrar/Ocultar la ayuda de las teclas de acceso rápido del teclado",
    "Focus client search box": "Enfocar el cuadro de búsqueda de cliente",
    "New Artifact Collection Wizard": "Asistente para nueva colección de artefactos",
    "Artifact Selection Step": "Paso de selección de artefactos",
    "Parameters configuration Step": "Paso de configuración de parámetros",
    "Collection resource specification": "Especificación de recursos de colección",
    "Launch artifact": "Lanzar artefacto",
    "Go to next step": "Ir al siguiente paso",
    "Go to previous step": "Ir al paso anterior",
    "Select next collection": "Seleccionar colección siguiente",
    "Select previous collection": "Seleccionar colección anterior",
    "View selected collection results": "Ver los resultados de la colección seleccionada",
    "View selected collection overview": "Ver descripción general de la colección seleccionada",
    "View selected collection logs": "Ver registros de la colección seleccionada",
    "View selected collection uploaded files": "Ver los archivos cargados de la colección seleccionada",
    "Editor shortcuts": "Accesos directos del editor",
    "Popup the editor configuration dialog": "Abrir el cuadro de diálogo de configuración del editor",
    "Save editor contents": "Guardar contenido del editor",
    "Keyboard shortcuts": "Métodos abreviados de teclado",
    "Yes do it!": "¡Sí, hazlo!",
    "Inspect Raw JSON": "Inspeccionar JSON sin procesar",
    "Raw Response JSON": "JSON de respuesta sin procesar",
    "Show/Hide Columns": "Mostrar/Ocultar Columnas",
    "Set All": "Establecer todo",
    "Clear All": "Borrar todo",
    "Exit Fullscreen": "Salir de pantalla completa",
    "Artifact Collection": "Colección de artefactos",
    "Uploaded Files": "Archivos cargados",
    "Results": "Resultados",
    "Flow Details": "Detalles de flujo",
    "Notebook for Collection": name=>"Bloc de notas para la colección "+name,
    "Please click a collection in the above table":"Haga clic en una colección de la tabla anterior",
    "Artifact Names": "Nombres de artefactos",
    "Creator": "Creador",
    "Create Time": "Hora de creación",
    "Start Time": "Hora de inicio",
    "Last Active": "Activo por última vez",
    "Duration": "Duración",
    " Running...": " Corriendo...",
    "State": "Estado",
    "Error": "Error",
    "CPU Limit": "Límite de CPU",
    "IOPS Limit": "Límite de IOPS",
    "Timeout": "Tiempo máximo de espera",
    "Max Rows": "Máximo de filas",
    "Max MB": "Máximo de MB",
    "Artifacts with Results": "Artefactos con resultados",
    "Total Rows": "Total de filas",
    "Uploaded Bytes": "Bytes cargados",
    "Files uploaded": "Archivos cargados",
    "Download Results": "Descargar Resultados",
    "Set a password in user preferences to lock the download file.": "Establecer una contraseña en las preferencias del usuario para bloquear el archivo de descarga.",
    "Prepare Download": "Preparar Descarga",
    "Prepare Collection Report": "Preparar informe de colección",
    "Available Downloads": "Descargas Disponibles",
    "Size (Mb)": "Tamaño (MB)",
    "Date": "Fecha",
    "Unlimited": "Ilimitado",
    "rows": "Filas",
    "Request sent to client": "Solicitud enviada al cliente",
    "Description": "Descripción",
    "Created": "Creado",
    "Manually add collection to hunt": "Agregar colección manualmente al hunt",
    "No compatible hunts.": "No hay hunts compatibles.",
    "Please create a hunt that collects one or more of the following artifacts.":"Por favor, cree un Hunt que recopile uno o más de los siguientes artefactos.",
    "Requests": "Peticiones",
    "Notebook": "Bloc de notas",
    "Permanently delete collection": "Eliminar colección de forma permanente",
    "ArtifactDeletionDialog": (session_id, artifacts, total_bytes, total_rows)=>
    <>
      Está a punto de eliminar de forma permanente la colección de
      artefactos <b>{session_id}</b>.
      <br/>
      Esta colección comprende los artefactos <b className="wrapped-text">
         {artifacts}
      </b>
      <br/><br/>

      Se espera liberar { total_bytes.toFixed(0) } MB  de
      datos y { total_rows } filas.
    </>,
    "Save this collection to your Favorites": "Guardar esta colección en favoritos",
    "ArtifactFavorites": artifacts=>
    <>
      Puede recuperar fácilmente la misma colección de los
      favoritos en el futuro.
      <br/>
      Esta colección comprendía los artefactos <b>{artifacts}</b>
      <br/><br/>
    </>,
    "New Favorite name": "Nuevo nombre de favorito",
    "Describe this favorite": "Descripción de favorito",
    "New Collection": "Nueva colección",
    "Add to hunt": "Agregar al Hunt",
    "Delete Artifact Collection": "Eliminar colección de artefactos",
    "Cancel Artifact Collection": "Cancelar colección de artefactos",
    "Copy Collection": "Copiar colección",
    "Save Collection": "Guardar colección",
    "Build offline collector": "Crear recopilador sin conexión",
    "Notebooks": "Blocs de notas",
    "Full Screen": "Pantalla completa",
    "Delete Notebook": "Eliminar bloc de notas",
    "Notebook Uploads": "Cargar bloc de notas",
    "Export Notebook": "Exportar bloc de notas",
    "FINISHED": "FINALIZADO",
    "RUNNING": "EN EJECUCIÓN",
    "STOPPED": "DETENIDO",
    "PAUSED": "EN PAUSA",
    "ERROR": "ERROR",
    "CANCELLED": "CANCELADO",
    "Search for artifacts...": "Buscar artefactos...",
    "Favorite Name": "Nombre de favorito",
    "Artifact": "Artefacto",
    "No artifacts configured. Please add some artifacts to collect": "No hay artefactos configurados. Agregue algunos artefactos para recopilar",

    "Artifacts": "Artefactos",
    "Collected Artifacts": "Artefactos recopilados",
    "Flow ID": "ID de flujo",
    "FlowId": "IdFlujo",
    "Goto notebooks": "Ir a bloc de notas",
    "Max Mb": "Max Mb",
    "Mb": "Mb",
    "Name": "Nombre",
    "Ops/Sec": "Ops/Seg",
    "Rows": "Filas",
    "New Collection: Select Artifacts to collect":"Nueva colección: seleccione artefactos para recopilar",
    "Select Artifacts":"Seleccionar Artefactos",
    "Configure Parameters":"Configurar parámetros",
    "Specify Resources":"Especificar Recursos",
    "Review":"Revisar",
    "Launch":"Iniciar",
    "New Collection: Configure Parameters":"Nueva Colección: Configurar Parámetros",
    "New Collection: Specify Resources":"Nueva Colección: Especificar Recursos",
    "New Collection: Review request":"Nueva Colección: Solicitud de revisión",
    "New Collection: Launch collection":"Nueva colección: Lanzar colección",

    "CPU Limit Percent":"Porcentaje de límite de CPU",
    "IOps/Sec":"IOps/Seg",
    "Max Execution Time in Seconds":"Tiempo Max. de ejecución en segundos",
    "Max Idle Time in Seconds":"Tiempo Máximo de Inactividad en Segundos",
    "If set collection will be terminated after this many seconds with no progress.":"Si se encuentra configurado, la recopilación finalizará después de estos segundos sin que haya habido progreso.",
    "Max Mb Uploaded":"Máx. MB subidos",
    "Collection did not upload files":"La colección no subió ningún archivo",

    "Create Offline collector: Select artifacts to collect":"Crear recopilador sin conexión: seleccionar artefactos para recopilar",
    "Configure Collection":"Configurar Colección",
    "Create Offline Collector: Configure artifact parameters":"Crear recopilador sin conexión: configurar parámetros de artefactos",
    "Create Offline Collector: Review request":"Crear recopilador sin conexión: solicitud de revisión",
    "Create Offline Collector: Create collector":"Crear recopilador sin conexión: Crear recopilador",

    "Create Offline collector:  Configure Collector":"Crear recopilador sin conexión: Configurar recopilador",
    "Target Operating System":"Sistema operativo de destino",
    "Password":"Contraseña",
    "Report Template":"Plantilla de informe",
    "No Report":"Sin informe",
    "Collection Type":"Tipo de colección",
    "Zip Archive":"Archivo comprimido",
    "Google Cloud Bucket":"Bucket de Google Cloud",
    "AWS Bucket":"Bucket de AWS",
    "SFTP Upload":"Subida mediante SFTP",
    "Velociraptor Binary":"Ejecutable de Velociraptor",
    "Temp directory":"Directorio temporal",
    "Temp location":"Ubicación temporal",
    "Compression Level":"Nivel de Compresión",
    "Output format":"Formato de salida",
    "CSV and JSON":"CSV y JSON",
    "Output Prefix":"Prefijo de salida",
    "Output filename prefix":"Prefijo de nombre de archivo de salida",

    "DeleteHuntDialog": <>
                    <p>Está a punto de detener permanentemente y eliminar todos los datos de esta búsqueda.</p>
                    <p>¿Está seguro de que desea cancelar esta búsqueda y eliminar los datos recopilados?</p>
                        </>,

    "Started":"Iniciado",
    "Expires":"Caduca",
    "Scheduled":"Programado",
    "New Hunt":"Nuevo Hunt",
    "Run Hunt":"Ejecutar Hunt",
    "Stop Hunt":"Detener Hunt",
    "Delete Hunt":"Eliminar Hunt",
    "Copy Hunt":"Copiar Hunt",
    "No hunts exist in the system. You can start a new hunt by clicking the New Hunt button above.":"No hay búsquedas en el sistema. Puede iniciar una nueva búsqueda haciendo clic en el botón Nueva búsqueda. ",
    "Please select a hunt above":"Seleccione un Hunt",
    "Clients":"Clientes",
    "Notebook for Hunt": hunt_id=>"Bloc de notas para el Hunt " + hunt_id,

    "Hunt ID":"ID de Hunt",
    "Creation Time":"Tiempo de creación",
    "Expiry Time":"Tiempo de caducidad",
    "Total scheduled":"Total programado",
    "Finished clients":"Hosts finalizados",
    "Full Download":"Descarga completa",
    "Summary Download":"Descarga de resumen",
    "Summary (CSV Only)":"Resumen (Solo CSV)",
    "Summary (JSON Only)":"Resumen (solo JSON)",
    "name":"Nombre",
    "size":"tamaño",
    "date":"Fecha",
    "New Hunt - Configure Hunt":"Nuevo Hunt - Configurar Hunt",
    "Hunt description":"Descripción del Hunt",
    "Expiry":"Caducidad",
    "Include Condition":"Incluir condición",
    "Run everywhere":"Ejecutar en todas partes",
    "Exclude Condition":"Condición de exclusión",
    "Configure Hunt":"Configurar Hunt",
    "Estimated affected clients":"Estimación de hosts afectados",
    "All Known Clients":"Todos los hosts conocidos",
    "1 Day actives":"Activos 1 día",
    "1 Week actives":"Activos 1 semana",
    "1 Month actives":"Activos 1 mes",
    "Create Hunt: Select artifacts to collect":"Crear Hunt: Seleccionar artefactos para coleccionar",
    "Create Hunt: Configure artifact parameters":"Crear Hunt: Configurar parámetros de artefactos",
    "Create Hunt: Specify resource limits":"Crear Hunt: especificar límites de recursos",
    "Create Hunt: Review request":"Crear Hunt: Revisar petición",
    "Create Hunt: Launch hunt":"Crear Hunt: Lanzar Hunt",

    "ClientId": "IdCliente",
    "StartedTime":"Hora de inicio",
    "TotalBytes":"Total de bytes",
    "TotalRows":"Total de filas",

    "client_time":"Hora del host",
    "level":"nivel",
    "message":"mensaje",

    "RecursiveVFSMessage": path=><>
    Está a punto de obtener recursivamente todos los archivos en <b>{path}</b>.
    <br/><br/>
    Esto permite que se transfieran grandes cantidades de datos desde el equipo final. El límite de carga predeterminado es de 1 GB, pero puede cambiarlo en la pantalla Artefactos recopilados.
    </>,

    "Textview":"Vista en texto plano",
    "HexView":"Vista en Hexadecimal",
    "Refresh this directory (sync its listing with the client)":"Actualizar este directorio (sincronizar su listado con el host)",
    "Recursively refresh this directory (sync its listing with the client)":"Actualizar recursivamente este directorio (sincronizar su listado con el host)",
    "Recursively download this directory from the client":"Descargar recursivamente este directorio desde el host",
    "View Collection":"Ver colección",
    "Size":"Tamaño",
    "Mode":"modo",
    "mtime":"mtime",
    "atime":"atime",
    "ctime":"ctime",
    "btime":"btime",
    "Mtime":"Mtime",
    "Atime":"Atime",
    "Ctime":"Ctime",
    "Btime":"Btime",
    "Properties":"Propiedades",
    "No data available. Refresh directory from client by clicking above.":"No hay datos disponibles. Actualice el directorio del host haciendo clic arriba.",
    "Please select a file or a folder to see its details here.":"Seleccione un archivo o una carpeta para ver sus detalles aquí.",
    "Currently refreshing from the client":"Actualizando desde el host...",
    "Recursively download files":"Descargar archivos recursivamente",

    "Home":"Inicio",
    "Hunt Manager":"Administrador de Hunts",
    "View Artifacts":"Ver Artefactos",
    "Server Events":"Eventos del servidor",
    "Server Artifacts":"Artefactos del Servidor",
    "Host Information":"Información del host",
    "Virtual Filesystem":"Sistema de archivos virtual",
    "Client Events":"Eventos del Host",
    "This is a notebook for processing a hunt.":"Este es un bloc de notas para procesar un Hunt.",
    "ToolLocalDesc":
    <>
    La herramienta será proporcionada por el servidor de Velociraptor
    a los hosts si es necesario. El host
    almacenará en caché la herramienta en su propio disco y comparará el hash
    la próxima vez si es necesario. Las herramientas solo se descargarán si su
    hash ha cambiado.
    </>,
    "ServedFromURL": (url)=>
    <>
    Los hosts descargarán la herramienta directamente
    de <a href={api.href(url)}>{url}</a> si es
    necesario. Tenga en cuenta que si el hash no coincide con el
    hash esperado, los hosts rechazarán el archivo.
    </>,
    "ServedFromGithub": (github_project, github_asset_regex)=>
    <>
    La URL de la herramienta será actualizada por
    GitHub como la última versión del
    proyecto <b>{github_project}</b> que coincida
    con <b>{github_asset_regex}</b>
    </>,
    "PlaceHolder":
    <>
    El hash de la herramienta es desconocido. La primera vez
    que se necesite la herramienta, Velociraptor la descargará desde la
    URL y calculará su hash.
    </>,
    "ToolHash":
    <>
    Se calculó el hash de la herramienta. Cuando los hosts necesiten usar esta herramienta
    se asegurarán de que este hash coincida con lo que se descarga.
    </>,
    "AdminOverride":
    <>
    La herramienta fue subida manualmente por un
    administrador - no será actualiza automáticamente con la
    próxima actualización del servidor Velociraptor.
    </>,
    "ToolError":
    <>
    Se desconoce el hash de la herramienta y no se ha definido una URL.
    Será imposible emplear esta herramienta en un
    Artefacto ya que Velociraptor no puede resolverlo.
    Como alternativa, puede cargar un archivo manualmente.
    </>,
    "OverrideToolDesc":
    <>
    Como administrador, puede cargar manualmente un archivo
    ejecutable para ser utilizado como herramienta. Esto sobreescribirá la
    configuración de URL, y proporcionará la herramienta a todos aquellos
    Artefactos que lo necesitan. Como alternativa, puede configurar una URL para que
    los hosts puedan obtener herramientas.
    </>,

    "Include Labels":"Incluir Etiquetas",
    "Exclude Labels":"Excluir Etiquetas",
    "? for suggestions":"? para sugerencias",
    "Served from URL":"Proporcionado desde la URL",
    "Placeholder Definition":"Definición temporal",
    "Materialize Hash":"Materializar hash",
    "Tool":"Herramienta",
    "Override Tool":"Anular Herramienta",
    "Select file":"Seleccionar archivo",
    "Click to upload file":"Haga clic para subir archivo",
    "Set Serve URL":"Establecer URL para servir archivos",
    "Served Locally":"Archivos servidos localmente",
    "Tool Hash Known":"Hash de herramienta conocido",
    "Re-Download File":"Descargar archivo de nuevo",
    'Re-Collect from the client': "Volver a recopilar desde el host",
    'Collect from the client': 'Recopilar desde el host',
    "Tool Name":"Nombre de la herramienta",
    "Upstream URL":"URL",
    "Endpoint Filename":"nombre de archivo en el equipo destino",
    "Hash":"Hash",
    "Serve Locally":"Servir archivo localmente",
    "Serve URL":"Servir archivo desde URL",
    "Fetch from Client": "Obtener del host",
    "Last Collected": "Recopilado por última vez",
    "Offset": "desplazamiento",
    "Show All": "Mostrar todo",
    "Recent Hosts": "Hosts recientes",
    "Download JSON": "Descargar JSON",
    "Download CSV": "Descargar CSV",
    "Transform Table": "Tabla de transformación",
    "Transformed": "Transformado",

    "Select a notebook from the list above.":"Seleccione un bloc de notas de la lista.",
    "Cancel":"Cancelar",
    "Recalculate":"Recalcular",
    "Stop Calculating":"Dejar de calcular",
    "Edit Cell":"Editar Celda",
    "Up Cell":"Celda arriba",
    "Down Cell":"Celda abajo",
    "Add Cell":"Agregar celda",
    "Suggestion":"Sugerencia",
    "Suggestions":"Sugerencias",
    "Add Timeline":"Agregar línea de tiempo",
    "Add Cell From This Cell":"Agregar celda desde esta celda",
    "Add Cell From Hunt":"Agregar Celda desde un Hunt",
    "Add Cell From Flow":"Agregar celda desde un flujo",
    "Rendered":"Renderizado",
    "Undo":"Deshacer",
    "Delete Cell":"Eliminar Celda",
    "Uptime":"Tiempo activo",
    "BootTime":"Hora de arranque",
    "Procs":"Procesos",
    "OS":"Sistema operativo",
    "Platform":"Plataforma",
    "PlatformFamily":"Familia de plataforma",
    "PlatformVersion":"Versión de la plataforma",
    "KernelVersion":"Versión del kernel",
    "VirtualizationSystem":"Sistema de virtualización",
    "VirtualizationRole":"Rol de virtualización",
    "HostID":"ID de host",
    "Exe":"Exe",
    "Fqdn":"Fqdn",
    "Create a new Notebook":"Crear un nuevo bloc de notas",
    "Collaborators":"Colaboradores",
    "Submit":"Enviar",
    "Edit notebook ":"Editar bloc de notas ",
    "Notebook uploads":"Subir bloc de notas",
    "User Settings":"Configuración de usuario",
    "Select a user": "Seleccionar un usuario",

    "Theme":"Tema",
    "Select a theme":"Seleccionar un tema",
    "Default Velociraptor":"Velociraptor predeterminado",
    "Velociraptor Classic (light)": "Velociraptor Clásico (claro)",
    "Velociraptor (light)":"Velociraptor (claro)",
    "Ncurses (light)":"Ncurses (claro)",
    "Ncurses (dark)":"Ncurses (oscuro)",
    "Velociraptor (dark)":"Velociraptor (oscuro)",
    "Github dimmed (dark)":"Github atenuado (oscuro)",
    "Github (light)":"Github (claro)",
    "Cool Gray (dark)":"Gris frío (oscuro)",
    "Strawberry Milkshake (light)":"Batido de Fresa (claro)",
    "Downloads Password":"Contraseña de descarga",
    "Default password to use for downloads":"Contraseña predeterminada para usar en las descargas",

    "Create Artifact from VQL":"Crear artefacto desde VQL",
    "Member":"Miembro",
    "Response":"Respuesta",
    "Super Timeline":"Super línea de tiempo",
    "Super-timeline name":"Nombre de la súper línea de tiempo",
    "Timeline Name":"Nombre de línea de tiempo",
    "Child timeline name":"Nombre de la línea de tiempo secundaria",
    "Time column":"Columna de tiempo",
    "Time Column":"Columna de tiempo",
    "Language": "Idioma",
    "Match by label": "Coincidencia por etiqueta",
    "All known Clients": "Todos los hosts conocidos",
    "X per second": x=><>{x} por segundo</>,
    "HumanizeDuration": difference=>{
        if (difference<0) {
            return <>
                     En {humanizeDuration(difference, {
                         round: true,
                         language: "es",
                     })}
                   </>;
        }
        return <>
                 Antes de {humanizeDuration(difference, {
                     round: true,
                     language: "es",
                 })}
               </>;
    },
    "Transform table": "Tabla de transformación",
    "Sort Column": "Ordenar columna",
    "Filter Regex": "Filtrar Regex",
    "Filter Column": "Filtrar columna",
    "Select label to edit its event monitoring table": "Seleccione una etiqueta para editar su tabla de monitorización de eventos",
    "EventMonitoringCard":
    <>
    La monitorización de eventos se enfoca a grupos de etiquetas específicos.
    Seleccione un grupo de etiquetas para configurar
    artefactos de eventos específicos dirigidos a este grupo.
    </>,
    "Event Monitoring: Configure Label groups": "Monitorización de eventos: configurar grupos de etiquetas",
    "Configuring Label": "Configurando Etiqueta",
    "Event Monitoring Label Groups": "Grupos de Etiquetas de monitorización de Eventos",
    "Event Monitoring: Select artifacts to collect from label group ": "Monitorización de eventos: seleccione artefactos para recopilar del grupo de etiquetas ",
    "Artifact Collected": "Artefacto Recolectado",
    "Event Monitoring: Configure artifact parameters for label group ": "Monitorización de eventos: configurar parámetros de artefactos para el grupo de etiquetas ",
    "Event Monitoring: Review new event tables": "Monitorización de eventos: Revisar nuevas tablas de eventos",

    "Server Event Monitoring: Select artifacts to collect on the server":"Monitorización de eventos del servidor: seleccione los artefactos para recopilar en el servidor",
    "Server Event Monitoring: Configure artifact parameters for server":"Monitorización de eventos del servidor: Configurar parámetros de artefactos para el servidor",
    "Server Event Monitoring: Review new event tables":"Monitorización de eventos del servidor: revisar nuevas tablas de eventos",
    "Configure Label Group":"Configurar grupo de etiquetas",
    "Select artifact": "Seleccionar artefacto",

    "Raw Data":"Datos sin procesar",
    "Logs":"Logs",
    "Log":"Log",
    "Report":"Informe",

    "NotebookId":"ID del bloc de notas",
    "Modified Time":"Hora de modificación",
    "Time": "Hora",
    "No events": "Sin eventos",
    "_ts": "Hora del servidor",

    "Timestamp":"Timestamp",
    "started":"Iniciado en",
    "vfs_path":"Ruta VFS",
    "file_size":"Tamaño del archivo",
    "uploaded_size":"Tamaño de la subida",

    "Select a language":"Seleccione un idioma",
    "English":"Inglés",
    "Deutsch":"Alemán",
    "Spanish": "Español",
    "Portuguese": "Portugués",
    "French": "Francés",
    "Japanese": "Japonés",

    "Type":"Tipo",
    "Export notebooks":"Exportar bloc de notas",
    "Export to HTML":"Exportar a HTML",
    "Export to Zip":"Exportar a Zip",

    "Permanently delete Notebook":"Eliminar bloc de notas permanentemente",
    "You are about to permanently delete the notebook for this hunt":"Está a punto de eliminar de forma permanente el bloc de notas correspondiente a este Hunt",

    "Data":"Datos",
    "Served from GitHub":"Servido desde GitHub",
    "Refresh Github":"Actualizar desde GitHub",
    "Github Project":"Proyecto de GitHub",
    "Github Asset Regex":"Expresión regular de activo en Github",
    "Admin Override":"Sobrecarga de administrador",
    "Serve from upstream":"Servir fichero desde localización de subida",

    "Update server monitoring table":"Actualizar tabla de monitorización del servidor",
    "Show server monitoring tables":"Mostrar tablas de monitorización del servidor",

    "Display timezone": "Mostrar zona horaria",
    "Select a timezone": "Seleccione una zona horaria",

    "Update client monitoring table":"Actualizar tabla de monitorización de hosts",
    "Show client monitoring tables":"Mostrar tablas de monitorización de hosts",
    "Urgent": "Urgente",
    "Skip queues and run query urgently": "Omita las colas y ejecute la consulta con urgencia",

    // Below need verification
    "Role_administrator": "Administrador del servidor",
     "Role_org_admin": "Administrador de la organización",
     "Role_reader": "Usuario de solo lectura",
     "Role_analyst": "Analista",
     "Role_investigator": "Investigador",
     "Role_artifact_writer": "Escritor de artefactos",
     "Role_api": "Cliente API de solo lectura",

     "Perm_ANY_QUERY" : "Cualquier consulta",
     "Perm_PUBISH": "Publicar",
     "Perm_READ_RESULTS": "Leer resultados",
     "Perm_LABEL_CLIENT" : "Etiquetar Clientes",
     "Perm_COLLECT_CLIENT": "Recopilar cliente",
     "Perm_START_HUNT": "Iniciar búsqueda",
     "Perm_COLLECT_SERVER": "Recopilar servidor",
     "Perm_ARTIFACT_WRITER": "Escritor de artefactos",
     "Perm_SERVER_ARTIFACT_WRITER": "Escritor de artefactos del servidor",
     "Perm_EXECVE" : "EXECVE",
     "Perm_NOTEBOOK_EDITOR": "Editor de cuadernos",
     "Perm_SERVER_ADMIN": "Administrador del servidor",
     "Perm_ORG_ADMIN": "Administrador de la organización",
     "Perm_IMPERSONATION" : "Suplantación de identidad",
     "Perm_FILESYSTEM_READ": "Lectura del sistema de archivos",
     "Perm_FILESYSTEM_WRITE": "Escritura del sistema de archivos",
     "Perm_MACHINE_STATE": "Estado de la máquina",
     "Perm_PREPARE_RESULTS": "Preparar resultados",
     "Perm_DATASTORE_ACCESS": "Acceso al almacén de datos",

     "ToolPerm_ANY_QUERY": "Emitir cualquier consulta",
     "ToolPerm_PUBISH": "Publicar eventos en las colas del lado del servidor (normalmente no es necesario)",
     "ToolPerm_READ_RESULTS": "Leer resultados de búsquedas, flujos o cuadernos que ya se hayan ejecutado",
     "ToolPerm_LABEL_CLIENT": "Puede manipular etiquetas y metadatos de clientes",
     "ToolPerm_COLLECT_CLIENT": "Programar o cancelar nuevas colecciones en clientes",
     "ToolPerm_START_HUNT": "Iniciar una nueva búsqueda",
     "ToolPerm_COLLECT_SERVER": "Programar nuevas colecciones de artefactos en servidores Velociraptor",
     "ToolPerm_ARTIFACT_WRITER": "Agregar o editar artefactos personalizados que se ejecutan en el servidor",
     "ToolPerm_SERVER_ARTIFACT_WRITER": "Agregar o editar artefactos personalizados que se ejecutan en el servidor",
     "ToolPerm_EXECVE": "Permitido ejecutar comandos arbitrarios en clientes",
     "ToolPerm_NOTEBOOK_EDITOR": "Permitido cambiar cuadernos y celdas",
     "ToolPerm_SERVER_ADMIN": "Permitido para administrar la configuración del servidor",
     "ToolPerm_ORG_ADMIN": "Permitido para administrar organizaciones",
     "ToolPerm_IMPERSONATION": "Permite al usuario especificar un nombre de usuario diferente para el complemento de consulta()",
     "ToolPerm_FILESYSTEM_READ": "Permitido leer archivos arbitrarios del sistema de archivos",
     "ToolPerm_FILESYSTEM_WRITE": "Permitido para crear archivos en el sistema de archivos",
     "ToolPerm_MACHINE_STATE": "Permitido recopilar información de estado de las máquinas (por ejemplo, pslist())",
     "ToolPerm_PREPARE_RESULTS": "Permitido para crear archivos zip",
     "ToolPerm_DATASTORE_ACCESS": "Acceso permitido al almacén de datos sin procesar",
};

_.each(automated, (v, k)=>{
    Spanish[hex2a(k)] = v;
});

export default Spanish;
