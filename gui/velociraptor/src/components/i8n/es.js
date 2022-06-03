import Alert from 'react-bootstrap/Alert';
import humanizeDuration from "humanize-duration";

const Spanish = {
    SEARCH_CLIENTS: "Buscar Clientes",
    "Quarantine description": (<>
          <p>Está a punto de poner en cuarentena este host.</p>
          <p>
            Durante la cuarentena, el anfitrión no puede hacer esto
            comunicarse con todas las demás redes, excepto que
            Servidor Velociraptor.
          </p>
        </>),
    "Cannot Quarantine host": "No se puede poner en cuarentena el host",
    "Cannot Quarantine host message": (os_name, quarantine_artifact)=>
        <>
          <Alert variant="warning">
            { quarantine_artifact ?
              <p>Esta instancia de Velociraptor no tiene el artefacto <b>{quarantine_artifact}</b> necesario para poner en cuarentena los hosts que ejecutan {os_name}.</p> :
              <p>Esta instancia de Velociraptor no tiene un nombre de artefacto definido para los hosts en cuarentena que ejecutan {os_name}.</p>
            }
          </Alert>
        </>,
    "Client ID": "Identificación del cliente",
    "Agent Version": "Versión del agente",
    "Agent Name": "Nombre del Agente",
    "First Seen At": "Visto por primera vez en",
    "Last Seen At": "Visto por última vez en",
    "Last Seen IP": "Última IP vista",
    "Labels": "Etiquetas",
    "Operating System": "Sistema Operativo",
    "Hostname": "nombre de host",
    "FQDN": "FQDN",
    "Release": "Liberarg",
    "Architecture": "La Arquitectura",
    "Client Metadata": "Metadatos del cliente",
    "Interrogate": "Interroga",
    "VFS": "Sistema de archivos virtual",
    "Collected": "Recopilado",
    "Unquarantine Host": "Anfitrión sin cuarentena",
    "Quarantine Host": "Anfitrión en cuarentena",
    "Quarantine Message": "Mensaje de cuarentena",
    "Add Label": "Agregar etiqueta",
    "Overview": "Resumen",
    "VQL Drilldown": "Desglose",
    "Shell": "Carcasa",
    "Close": "Cerca",
    "Connected": "Conectado",
    "seconds": "segundos",
    "minutes": "minutos",
    "hours": "horas",
    "days": "días",
    "time_ago": function(value, unit) {
        unit = Spanish[unit] || unit;
        return 'Antes de ' + value + ' ' + unit;
    },
    "Online": "en línea",
    "Label Clients": "Etiqueta de Cliente",
    "Existing": "Existente",
    "A new label": "Una nueva etiqueta",
    "Add it!": "¡Añádelo!",
    "Delete Clients": "Eliminar Clientes",
    "DeleteMessage": "Está a punto de eliminar permanentemente los siguientes clientes",
    "Yeah do it!": "¡Sí, hazlo!",
    "Goto Page": "Ir a página",
    "Table is Empty": "La mesa está vacía",
    "OS Version": "Versión del sistema operativo",
    "Select a label": "Seleccionar una etiqueta",
    "Expand": "Expandir",
    "Collapse": "Colapsar",
    "Hide Output": "Ocultar salida",
    "Load Output": "Salida de carga",
    "Stop": "Para",
    "Delete": "Eliminar",
    "Run command on client": "Ejecutar comando en cliente",
    "Type VQL to run on the client": "Escriba VQL para ejecutar en el cliente",
    "Run VQL on client": "Ejecutar VQL en el cliente",
    "Artifact details": "Detalles del artefacto",
    "Artifact Name": "Nombre del artefacto",
    "Upload artifacts from a Zip pack": "Cargar artefactos desde un paquete Zip",
    "Select artifact pack (Zip file with YAML definitions)": "Seleccionar paquete de artefactos (archivo zip con definiciones YAML)",
    "Click to upload artifact pack file": "Haga clic para cargar el archivo del paquete de artefactos",
    "Delete an artifact": "Eliminar un artefacto",
    "You are about to delete": name=>"Está a punto de eliminar  " + name,
    "Add an Artifact": "Agregar un artefacto",
    "Edit an Artifact": "Editar un artefacto",
    "Delete Artifact": "Eliminar artefacto",
    "Hunt Artifact": "Caza Artefacto",
    "Collect Artifact": "Recoger Artefacto",
    "Upload Artifact Pack": "Cargar paquete de artefactos",
    "Search for artifact": "Buscar artefacto",
    "Search for an artifact to view it": "Buscar un artefacto para verlo",
    "Edit Artifact": name=>{
        return "Editar artefacto " + name;
    },
    "Create a new artifact": "Crear un nuevo artefacto",
    "Save": "Guardar",

    // Keyboard navigation.
    "Global hotkeys": "Teclas de acceso directo globales",
    "Goto dashboard": "Ir al tablero",
    "Collected artifacts": "Artefactos recopilados",
    "Show/Hide keyboard hotkeys help": "Mostrar/Ocultar la ayuda de las teclas de acceso rápido del teclado",
    "Focus client search box": "Cuadro de búsqueda de cliente Focus",
    "New Artifact Collection Wizard": "Asistente para nueva colección de artefactos",
    "Artifact Selection Step": "Paso de selección de artefactos",
    "Parameters configuration Step": "Paso de configuración de parámetros",
    "Collection resource specification": "Especificación de recurso de colección",
    "Launch artifact": "Lanzar artefacto",
    "Go to next step": "Ir al siguiente paso",
    "Go to previous step": "Ir al paso anterior",
    "Select next collection": "Seleccionar próximo sábadofuera de juego",
    "Select previous collection": "Seleccionar colección anterior",
    "View selected collection results": "Ver los resultados de la colección seleccionada",
    "View selected collection overview": "Ver descripción general de la colección seleccionada",
    "View selected collection logs": "Ver registros de recopilación seleccionados",
    "View selected collection uploaded files": "Ver los archivos cargados de la colección seleccionada",
    "Editor shortcuts": "Métodos abreviados del editor",
    "Popup the editor configuration dialog": "Abrir el cuadro de diálogo de configuración del editor",
    "Save editor contents": "Guardar contenido del editor",
    "Keyboard shortcuts": "Métodos abreviados de teclado",
    "Yes do it!": "¡Sí, hazlo!",
    "Inspect Raw JSON": "Inspeccionar JSON sin procesar",
    "Raw Response JSON": "Respuesta sin formato JSON",
    "Show/Hide Columns": "Mostrar/Ocultar Columnas",
    "Set All": "Establecer todo",
    "Clear All": "Borrar todo",
    "Exit Fullscreen": "Salir de pantalla completa",
    "Artifact Collection": "Colección de artefactos",
    "Uploaded Files": "Archivos cargados",
    "Results": "Resultados",
    "Flow Details": "Detalles de flujo",
    "Notebook for Collection": name=>"Cuaderno para colección "+name,
    "Please click a collection in the above table":"Haga clic en una colección de la tabla anterior",
    "Artifact Names": "Nombres de artefactos",
    "Creator": "Creador",
    "Create Time": "Crear Hora",
    "Start Time": "Hora de inicio",
    "Last Active": "Última actividad",
    "Duration": "Duración",
    " Running...": " Corriendo...",
    "State": "Estado",
    "Error": "Error",
    "CPU Limit": "Límites de CPU",
    "IOPS Limit": "Límites de IOPS",
    "Timeout": "Tiempo de espera",
    "Max Rows": "Filas máximas",
    "Max MB": "MB máximo",
    "Artifacts with Results": "Artefactos con resultados",
    "Total Rows": "Filas totales",
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
    "Manually add collection to hunt": "Agregar colección manualmente para cazar",
    "No compatible hunts.": "Cazas no compatibles.",
    "Please create a hunt that collects one or more of the following artifacts.":"Por favor, cree una cacería que recopile uno o más de los siguientes artefactos.",
    "Requests": "Solicitudes",
    "Notebook": "Cuaderno",
    "Permanently delete collection": "Eliminar colección de forma permanente",
    "ArtifactDeletionDialog": (session_id, artifacts, total_bytes, total_rows)=>
    <>
      Está a punto de eliminar de forma permanente la colección de artefactos
      <b>{session_id}</b>.
      <br/>
      Esta colección tenía los artefactos <b className="wrapped-text">
         {artifacts}
      </b>
      <br/><br/>

      Asumimos que liberamos { total_bytes.toFixed(0) } MB  de almacenamiento
      data y { total_rows } filas.
    </>,
    "Save this collection to your Favorites": "Guardar esta colección en tus favoritos",
    "ArtifactFavorites": artifacts=>
    <>
      Puedes coleccionar fácilmente la misma colección de la tuya
      favoritos en el futuro.
      <br/>
      Esta colección eran los <b>{artifacts}</b> artefactos
      <br/><br/>
    </>,
    "New Favorite name": "Nuevo nombre favorito",
    "Describe this favorite": "Describe este favorito",
    "New Collection": "Nueva colección",
    "Add to hunt": "Agregar a la caza",
    "Delete Artifact Collection": "Eliminar colección de artefactos",
    "Cancel Artifact Collection": "Cancelar colección de artefactos",
    "Copy Collection": "Copiar colección",
    "Save Collection": "Guardar colección",
    "Build offline collector": "Crear recopilador sin conexión",
    "Notebooks": "Cuadernos",
    "Full Screen": "Pantalla completa",
    "Delete Notebook": "Eliminar libreta",
    "Notebook Uploads": "Cargas de portátiles",
    "Export Notebook": "Exportar cuaderno",
    "FINISHED": "FINALIZADO",
    "RUNNING": "EN FUNCIONAMIENTO",
    "STOPPED": "DETENIDO",
    "PAUSED": "EN PAUSA",
    "ERROR": "ERROR",
    "CANCELLED": "Cancelado",
    "Search for artifacts...": "Buscar artefactos...",
    "Favorite Name": "Nombre favorito",
    "Artifact": "Artefacto",
    "No artifacts configured. Please add some artifacts to collect": "No hay artefactos configurados. Agregue algunos artefactos para recopilar",

    "Artifacts": "Artefactos",
    "Collected Artifacts": "Artefactos recopilados",
    "Flow ID": "ID de flujo",
    "FlowId": "IdFlujo",
    "Goto notebooks": "Ir a libretas",
    "Max Mb": "Max Mb",
    "Mb": "Mb",
    "Name": "Nombre",
    "Ops/Sec": "Ops/Seg",
    "Rows": "Filas",
    "New Collection: Select Artifacts to collect":"Nueva colección: seleccione artefactos para coleccionar",
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
    "If set collection will be terminated after this many seconds with no progress.":"Si la recopilación de conjuntos finalizará después de estos segundos sin progreso.",
    "Max Mb Uploaded":"Máx. MB subidos",
    "Collection did not upload files":"La colección no cargó archivos",

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
    "Google Cloud Bucket":"Cubo de Google Cloud",
    "AWS Bucket":"Cubo de AWS",
    "SFTP Upload":"Carga SFTP",
    "Velociraptor Binary":"Binario Velociraptor",
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
    "Scheduled":"Planificado",
    "New Hunt":"Nueva Cacería",
    "Run Hunt":"Ejecutar Caza",
    "Stop Hunt":"Detener la caza",
    "Delete Hunt":"Eliminar cacería",
    "Copy Hunt":"Búsqueda de copias",
    "No hunts exist in the system. You can start a new hunt by clicking the New Hunt button above.":"No hay búsquedas en el sistema. Puede iniciar una nueva búsqueda haciendo clic en el botón Nueva búsqueda arriba. ",
    "Please select a hunt above":"Seleccione una cacería arriba",
    "Clients":"Clientes",
    "Notebook for Hunt": hunt_id=>"Cuaderno para Hunt " + hunt_id,

    "Hunt ID":"ID de caza",
    "Creation Time":"Tiempo de creación",
    "Expiry Time":"Tiempo de caducidad",
    "Total scheduled":"Total programado",
    "Finished clients":"Clientes finalizados",
    "Full Download":"Descarga completa",
    "Summary Download":"Descarga resumida",
    "Summary (CSV Only)":"Resumen (Solo CSV)",
    "Summary (JSON Only)":"Resumen (solo JSON)",
    "name":"Nombre",
    "size":"tamaño",
    "date":"Fecha",
    "New Hunt - Configure Hunt":"Nueva Búsqueda - Configurar Búsqueda",
    "Hunt description":"Descripción de la caza",
    "Expiry":"Caducidad",
    "Include Condition":"Incluir condición",
    "Run everywhere":"Corre por todas partes",
    "Exclude Condition":"Excluir condición",
    "Configure Hunt":"Configurar Caza",
    "Estimated affected clients":"Clientes afectados estimados",
    "All Known Clients":"Todos los Clientes Conocidos",
    "1 Day actives":"Activos de 1 día",
    "1 Week actives":"Activos de 1 semana",
    "1 Month actives":"Activos de 1 mes",
    "Create Hunt: Select artifacts to collect":"Crear Caza: Seleccionar artefactos para coleccionar",
    "Create Hunt: Configure artifact parameters":"Crear Caza: Configurar parámetros de artefactos",
    "Create Hunt: Specify resource limits":"Crear búsqueda: especificar límites de recursos",
    "Create Hunt: Review request":"Crear Caza: Solicitud de revisión",
    "Create Hunt: Launch hunt":"Crear búsqueda: Iniciar búsqueda",

    "ClientId": "IdCliente",
    "StartedTime":"Hora de inicio",
    "TotalBytes":"Total de bytes",
    "TotalRows":"Total de filas",

    "client_time":"Hora del cliente",
    "level":"nivel",
    "message":"mensaje",

    "RecursiveVFSMessage": path=><>
    Está a punto de obtener recursivamente todos los archivos en <b>{path}</b>.
    <br/><br/>
    Esto permite que se transfieran grandes cantidades de datos desde el punto final. El límite de carga predeterminado es de 1 GB, pero puede cambiarlo en la pantalla Artefactos recopilados.
    </>,

    "Textview":"Vista de texto",
    "HexView":"HexView",
    "Refresh this directory (sync its listing with the client)":"Actualizar este directorio (sincronizar su listado con el cliente)",
    "Recursively refresh this directory (sync its listing with the client)":"Actualizar recursivamente este directorio (sincronizar su listado con el cliente)",
    "Recursively download this directory from the client":"Descargar recursivamente este directorio desde el cliente",
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
    "No data available. Refresh directory from client by clicking above.":"No hay datos disponibles. Actualice el directorio del cliente haciendo clic arriba.",
    "Please select a file or a folder to see its details here.":"Seleccione un archivo o una carpeta para ver sus detalles aquí.",
    "Currently refreshing from the client":"Actualmente actualizándose desde el cliente",
    "Recursively download files":"Descargar archivos recursivamente",

    "Home":"Hogar",
    "Hunt Manager":"Administrador de caza",
    "View Artifacts":"Ver Artefactos",
    "Server Events":"Eventos del servidor",
    "Server Artifacts":"Artefactos del Servidor",
    "Host Information":"Información del anfitrión",
    "Virtual Filesystem":"Sistema de archivos virtual",
    "Client Events":"Eventos del Cliente",
    "This is a notebook for processing a hunt.":"Este es un cuaderno para procesar una cacería.",
    "ToolLocalDesc":
    <>
    La herramienta es proporcionada por el servidor Velociraptor
    Clientes, si corresponde. El cliente
    Almacene en caché la herramienta en su propio disco y luego compare el hash
    tiempo que se necesita. Las herramientas solo se descargarán si su
    Hachís ha cambiado.
    </>,
    "ServedFromURL": (base_path, url)=>
    <>
    Los clientes obtienen la herramienta directamente de
    <a href={base_path + url}>{url}</a> si
    necesario. Tenga en cuenta que si el hash no coincide con el
    hash esperado, los clientes rechazarán el archivo.
    </>,
    "ServedFromGithub": (github_project, github_asset_regex)=>
    <>
    La URL de la herramienta es actualizada por
    GitHub como la última versión del proyecto
    <b>{github_project}</b>  que se ajuste
    <b>{github_asset_regex}</b>
    </>,
    "PlaceHolder":
    <>
    El hash de la herramienta es actualmente desconocido. La herramienta por primera vez
    es requerido, Velociraptor lo descargará desde allí
    url ascendente y calcular su hash.
    </>,
    "ToolHash":
    <>
    Se calculó el hash de la herramienta. Si los clientes necesitan usar
    Usan esta herramienta para asegurarse de que este hash coincida con lo que están haciendo.
    Descargar.
    </>,
    "AdminOverride":
    <>
    La herramienta fue cargada manualmente por un
    admin - no se actualiza automáticamente en el
    próxima actualización del servidor Velociraptor.
    </>,
    "ToolError":
    <>
    Se desconoce el hash de la herramienta y no una URL
    se define. Será imposible utilizar esta herramienta en una
    Artefacto porque Velociraptor no puede disiparlo. Tú
    puede cargar un archivo manualmente.
    </>,
    "OverrideToolDesc":
    <>
    Como administrador, puede cargar manualmente un archivo
    binary se puede utilizar como esta herramienta. Esto sobrescribe el
    configuración de URL ascendente y haga que su herramienta esté disponible para todos
    Artefactos que lo necesitan. Alternativamente, establezca una URL para clientes
    para conseguir herramientas.
    </>,

    "Include Labels":"Incluir Etiquetas",
    "Exclude Labels":"Excluir Etiquetas",
    "? for suggestions":"? para sugerencias",
    "Served from URL":"Servido por URL",
    "Placeholder Definition":"Definición de marcador de posición",
    "Materialize Hash":"Materializar hash",
    "Tool":"Herramienta",
    "Override Tool":"Anular Herramienta",
    "Select file":"Seleccionar archivo",
    "Click to upload file":"Haga clic para cargar el archivo",
    "Set Serve URL":"Establecer URL de servidor",
    "Served Locally":"Servido localmente",
    "Tool Hash Known":"Hash de herramienta conocido",
    "Re-Download File":"Descargar archivo de nuevo",
    'Re-Collect from the client': "Recobrar del cliente",
    'Collect from the client': 'Cobrar del cliente',
    "Tool Name":"Nombre de la herramienta",
    "Upstream URL":"URL ascendente",
    "Enpoint Filename":"enpoint filename",
    "Hash":"Hash",
    "Serve Locally":"Servir localmente",
    "Serve URL":"Servir URL",
    "Fetch from Client": "Obtener del cliente",
    "Last Collected": "Última recogida",
    "Offset": "desplazamiento",
    "Show All": "Mostrar todo",
    "Recent Hosts": "Hosts recientes",
    "Download JSON": "Descargar JSON",
    "Download CSV": "Descargar archivo CSV",
    "Transform Table": "Tabla de transformación",
    "Transformed": "Transformado",

    "Select a notebook from the list above.":"Seleccione una libreta de la lista anterior.",
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
    "Add Cell From Hunt":"Agregar Celda de Búsqueda",
    "Add Cell From Flow":"Agregar celda desde flujo",
    "Rendered":"Rendido",
    "Undo":"Deshacer",
    "Delete Cell":"Eliminar Celda",
    "Uptime":"Disponibilidad",
    "BootTime":"BootTime",
    "Procs":"Procesos",
    "OS":"Sistema operativo",
    "Platform":"Plataforma",
    "PlatformFamily":"Familia de plataforma",
    "PlatformVersion":"Versión de la plataforma",
    "KernelVersion":"KernelVersion",
    "VirtualizationSystem":"Sistema de virtualización",
    "VirtualizationRole":"VirtualizationRole",
    "HostID":"ID de host",
    "Exe":"Exe",
    "Fqdn":"Fqdn",
    "Create a new Notebook":"Crear un nuevo cuaderno",
    "Collaborators":"Empleados",
    "Submit":"Enviar",
    "Edit notebook ":"Editar libreta ",
    "Notebook uploads":"Cargas de cuadernos",
    "User Settings":"Configuración de usuario",
    "Select a user": "Seleccionar un usuario",

    "Theme":"Tema",
    "Select a theme":"Seleccionar un tema",
    "Default Velociraptor":"Velociraptor predeterminado",
    "Velociraptor (light)":"Velociraptor (luz)",
    "Ncurses (light)":"Ncurses (luz)",
    "Velociraptor (dark)":"Velociraptor (oscuro)",
    "Github dimmed (dark)":"Github atenuado (oscuro)",
    "Cool Gray (dark)":"Gris frío (oscuro)",
    "Strawberry Milkshake (light)":"Batido de Fresa (light)",
    "Downloads Password":"Contraseña de descarga",
    "Default password to use for downloads":"Contraseña predeterminada para usar en las descargas",

    "Create Artifact from VQL":"Crear artefacto desde VQL",
    "Member":"Miembro",
    "Response":"Respuesta",
    "Super Timeline":"Gran línea de tiempo",
    "Super-timeline name":"Nombre de la súper línea de tiempo",
    "Timeline Name":"Nombre de la columna de tiempo",
    "Child timeline name":"Name Nombre de la línea de tiempo secundaria",
    "Time column":"Columna de tiempo",
    "Time Column":"Columna de tiempo",
    "Language": "Idioma",
    "Match by label": "Coincidencia por etiqueta",
    "All known Clients": "Todos los Clientes conocidos",
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
    "Filter Column": "Columna de filtro",
    "Select label to edit its event monitoring table": "Seleccione la etiqueta para editar su tabla de monitoreo de eventos",
    "EventMonitoringCard":
    <>
    El monitoreo de eventos se dirige a grupos de etiquetas específicos.
    Seleccione un grupo de etiquetas arriba para configurarlo específicamente
    Artefactos de eventos dirigidos a este grupo.
    </>,
    "Event Monitoring: Configure Label groups": "Monitoreo de eventos: configurar grupos de etiquetas",
    "Configuring Label": "Configurando Etiqueta",
    "Event Monitoring Label Groups": "Grupos de Etiquetas de Monitoreo de Eventos",
    "Event Monitoring: Select artifacts to collect from label group ": "Monitoreo de eventos: seleccione artefactos para recopilar del grupo de etiquetas ",
    "Artifact Collected": "Artefacto Recolectado",
    "Event Monitoring: Configure artifact parameters for label group ": "Monitoreo de eventos: configurar parámetros de artefactos para el grupo de etiquetas ",
    "Event Monitoring: Review new event tables": "Monitoreo de eventos: Revisar nuevas tablas de eventos",

    "Server Event Monitoring: Select artifacts to collect on the server":"Supervisión de eventos del servidor: seleccione los artefactos para recopilar en el servidor",
    "Server Event Monitoring: Configure artifact parameters for server":"Supervisión de eventos del servidor: Configurar parámetros de artefactos para el servidor",
    "Server Event Monitoring: Review new event tables":"Supervisión de eventos del servidor: revisar nuevas tablas de eventos",
    "Configure Label Group":"Configurar grupo de etiquetas",
    "Select artifact": "Seleccionar artefacto",

    "Raw Data":"Datos sin procesar",
    "Logs":"Archivo de registro",
    "Log":"Archivo de registro",
    "Report":"Informe",

    "NotebookId":"Identificación del cuaderno",
    "Modified Time":"Hora Modificada",
    "Time": "Tiempo",
    "No events": "Sin eventos",
    "_ts": "Hora del servidor",

    "Timestamp":"Marca de tiempo",
    "started":"Iniciado",
    "vfs_path":"Ruta VFS",
    "file_size":"Tamaño del archivo",
    "uploaded_size":"Tamaño subido",
    "TablePagination": (from, to, size)=>
    <>Mostrar línea { from } a { to } de { size }</>,

    "Select a language":"Seleccione un idioma",
    "English":"Inglés",
    "Deutsch":"Alemán",
    "Spanish": "Español",
    "Portuguese": "Portugués",

    "Type":"Tipo",
    "Export notebooks":"Exportar cuadernos",
    "Export to HTML":"Exportar a HTML",
    "Export to Zip":"Nach Exportar a Zip",

    "Permanently delete Notebook":"Eliminar permanentemente Notebook",
    "You are about to permanently delete the notebook for this hunt":"Está a punto de eliminar de forma permanente la libreta de esta búsqueda",

    "Data":"Datos",
    "Served from GitHub":"Servido por GitHub",
    "Refresh Github":"Actualizar desde GitHub",
    "Github Project":"Proyecto GitHub",
    "Github Asset Regex":"Github Activo Regex",
    "Admin Override":"Anulación de administrador",
    "Serve from upstream":"Servir desde arriba",

    "Update server monitoring table":"Actualizar tabla de monitoreo del servidor",
    "Show server monitoring tables":"Mostrar tablas de monitoreo del servidor",

    "Display timezone": "Mostrar zona horaria",
    "Select a timezone": "Seleccione una zona horaria",

    "Update client monitoring table":"Actualizar tabla de seguimiento de clientes",
    "Show client monitoring tables":"Mostrar tablas de seguimiento de clientes",

};

export default Spanish;
