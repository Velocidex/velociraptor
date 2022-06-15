import Alert from 'react-bootstrap/Alert';
import humanizeDuration from "humanize-duration";

const Portuguese = {
    SEARCH_CLIENTS: "Pesquisar clientes",
    "Quarantine description": (
        <>
          <p>Você está prestes a colocar este host em quarentena.</p>
          <p>
            Durante a quarentena, o host não pode fazer isso
            comunicar-se com todas as outras redes, exceto aquela
            Servidor Velociraptor.
          </p>
        </>),
    "Cannot Quarantine host": "Não foi possível colocar o host em quarentena",
    "Cannot Quarantine host message": (os_name, quarantine_artifact)=>
    <>
      <Alert variant="warning">
        { quarantine_artifact ?
          <p>Esta instância do Velociraptor não tem o artefato <b>{quarantine_artifact}</b> necessário para colocar em quarentena hosts que executam {os_name}.</p> :
          <p>Esta instância do Velociraptor não tem um nome de artefato definido para colocar em quarentena os hosts que executam {os_name}.</p>
        }
      </Alert>
    </>,
    "Client ID": "ClientID",
    "Agent Version": "Versão do agente",
    "Agent Name": "Nome do agente",
    "First Seen At": "Visto pela primeira vez em",
    "Last Seen At": "Visto pela última vez em",
    "Last Seen IP": "IP visto pela última vez",
    "Labels": "Etiquetas",
    "Operating System": "Sistema operacional",
    "Hostname": "Nome do host",
    "FQDN": "FQDN",
    "Release": "Liberar",
    "Architecture": "Arquitetura",
    "Client Metadata": "Metadados do cliente",
    "Interrogate": "Consultas",
    "VFS": "Sistema de arquivos virtuais",
    "Collected": "Coletado",
    "Unquarantine Host": "Host fora da quarentena",
    "Quarantine Host": "Anfitrião de quarentena",
    "Quarantine Message": "Mensagem de quarentena",
    "Add Label": "Adicionar etiqueta",
    "Overview": "Visão geral",
    "VQL Drilldown": "Expandir",
    "Shell": "Concha",
    "Close": "Fechar",
    "Connected": "Conectado",
    "seconds": "Segundos",
    "minutes": "Minutos",
    "hours": "Horas",
    "days": "Dias",
    "time_ago":  function(value, unit) {
        unit = Portuguese[unit] || unit;
        return 'Antes' + value + ' ' + unit;
    },
    "Online": "On-line",
    "Label Clients": "Etiqueta do cliente",
    "Existing": "Existente",
    "A new label": "Um novo rótulo",
    "Add it!": "Adicione!",
    "Delete Clients": "Excluir clientes",
    "DeleteMessage": "Você está prestes a excluir permanentemente os seguintes clientes",
    "Yeah do it!": "Sim, faça isso!",
    "Goto Page": "Ir para a página",
    "Table is Empty": "A tabela está vazia",
    "OS Version": "Versão do sistema operacional",
    "Select a label": "Selecione um rótulo",
    "Expand": "Expandir",
    "Collapse": "Detalhamento",
    "Hide Output": "Ocultar desempenho",
    "Load Output": "Carregar desempenho",
    "Stop": "Parar",
    "Delete": "Excluir",
    "Run command on client": "Executar comando no cliente",
    "Type VQL to run on the client": "Digite VQL para executar no cliente",
    "Run VQL on client": "Executar VQL no cliente",
    "Artifact details": "Detalhes do artefato",
    "Artifact Name": "Nome do artefato",
    "Upload artifacts from a Zip pack": "Carregar artefatos de um pacote zip",
    "Select artifact pack (Zip file with YAML definitions)": "Selecionar pacote de artefatos (arquivo zip com YAML)",
    "Click to upload artifact pack file": "Clique aqui para fazer upload do arquivo do pacote de artefatos",
    "Delete an artifact": "Excluir um artefato",
    "You are about to delete": name=>"Você está prestes a excluir " + name,
    "Add an Artifact": "Adicionar um artefato",
    "Edit an Artifact": "Editar um artefato",
    "Delete Artifact": "Excluir artefato",
    "Hunt Artifact": "Artefato de caça",
    "Collect Artifact": "Coletar Artefato",
    "Upload Artifact Pack": "Carregar pacote de artefatos",
    "Search for artifact": "Encontrar artefato",
    "Search for an artifact to view it": "Procure um artefato para visualizá-lo",
    "Edit Artifact": nome=>{
        return "Editar artefato" + nome;
    },
    "Create a new artifact": "Criar um novo artefato",
    "Save": "Salvar",

    // Navegação do teclado.
    "Global hotkeys": "Teclas de atalho globais",
    "Goto dashboard": "Ir para o Painel",
    "Collected artifacts": "Artefatos Coletados",
    "Show/Hide keyboard hotkeys help": "Mostrar/ocultar a ajuda das teclas de atalho do teclado",
    "Focus client search box": "Foco na caixa de pesquisa do cliente",
    "New Artifact Collection Wizard": "Assistente de nova coleção de artefatos",
    "Artifact Selection Step": "Etapa de seleção de artefato",
    "Parameters configuration Step": "Etapa de configuração de parâmetros",
    "Collection resource specification": "Capture Resource Specification",
    "Launch artifact": "Iniciar artefato",
    "Go to next step": "Ir para a próxima etapa",
    "Go to previous step": "Ir para a etapa anterior",
    "Select next collection": "Selecione a próxima coleção",
    "Select previous collection": "Selecionar coleção anterior",
    "View selected collection results": "Mostrar resultados de coleta selecionados",
    "View selected collection overview": "Mostrar visão geral da coleção selecionada",
    "View selected collection logs": "Ver logs de coleta selecionados",
    "View selected collection uploaded files": "Visualizar arquivos de coleção enviados selecionados",
    "Editor shortcuts": "Atalhos do Editor",
    "Popup the editor configuration dialog": "Abrir caixa de diálogo de configuração do Bloco de Notas",
    "Save editor contents": "Salvar conteúdo do editor",
    "Keyboard shortcuts": "Atalho de teclado",
    "Yes do it!": "Sim, faça isso!",
    "Inspect Raw JSON": "Verificar JSON bruto",
    "Raw Response JSON": "JSON de resposta não formatada",
    "Show/Hide Columns": "Mostrar/Ocultar Colunas",
    "Set All": "Definir tudo",
    "Clear All": "Excluir tudo",
    "Exit Fullscreen": "Sair da tela inteira",
    "Artifact Collection": "Coleção de Artefatos",
    "Uploaded Files": "Arquivos enviados",
    "Results": "Resultados",
    "Flow Details": "Detalhes do fluxo",
    "Notebook for Collection": name=>"Caderno da coleção "+name,
    "Please click a collection in the above table":"Clique em uma coleção na tabela acima",
    "Artifact Names": "Nomes de artefatos",
    "Creator": "Criador",
    "Create Time": "Ganhar tempo",
    "Start Time": "Hora de início",
    "Last Active": "Atividade recente",
    "Duration": "Duração",
    " Running...": "Operando...",
    "State": "Estado",
    "Error": "Erro",
    "CPU Limit": "Limites de CPU",
    "IOPS Limit": "Limites de IOPS",
    "Timeout": "Tempo limite",
    "Max Rows": "Máximo de linhas",
    "Max MB": "Máx. MB",
    "Artifacts with Results": "Artefatos com resultados",
    "Total Rows": "Total de Linhas",
    "Uploaded Bytes": "Bytes enviados",
    "Files uploaded": "Arquivos enviados",
    "Download Results": "Baixar resultados",
    "Set a password in user preferences to lock the download fences to lock the download file.": "Para bloquear o download.",
    "Prepare Download": "Preparar download",
    "Prepare Collection Report": "Criar um relatório de resumo",
    "Available Downloads": "Downloads disponíveis",
    "Size (Mb)": "Tamanho (MB)",
    "Date": "Data",
    "Unlimited": "Ilimitado",
    "rows": "Linhas",
    "Request sent to client": "Solicitação enviada ao cliente",
    "Description": "Descrição",
    "Created": "Criado",
    "Manually add collection to hunt": "Adicionar coleção manualmente para caçar",
    "No compatible hunts.": "Nenhuma caça compatível.",
    "Please create a hunt that collects one or more of the following artifacts.":"Bitte erstellen Sie eine Jagd, die eines oder mehrere der folgenden Artefakte sammelt.",
    "Requests": "Solicitações",
    "Notebook": "Caderno",
    "Permanently delete collection": "Excluir coleção permanentemente",
    "ArtifactDeletionDialog": (session_id, artifacts, total_bytes, total_rows)=>
    <>
      Você está prestes a excluir permanentemente a coleção de artefatos
      <b>{session_id}</b>.
      <br/>
      Esta coleção tinha os artefatos <b className="wrapped-text">
                    {artifacts}
      </b>
      <br/><br/>

      Presumimos liberar { total_bytes.toFixed(0) } MB de armazenamento
      dados e { total_rows } linhas.
    </>,
    "Save this collection to your Favorites": "Salvar esta coleção em seus favoritos",
    "ArtifactFavorites": artifacts=>
    <>
      Você pode coletar facilmente a mesma coleção da sua
      favoritos no futuro.
      <br/>
      Esta coleção foi os artefatos <b>{artifacts}</b>
      <br/><br/>
    </>,
    "New Favorite name": "Novo nome favorito",
    "Describe this favorite": "Descreva este favorito",
    "New Collection": "Nova coleção",
    "Add to hunt": "Adicionar à caça",
    "Delete Artifact Collection": "Excluir coleção de artefatos",
    "Cancel Artifact Collection": "Cancelar coleção de artefatos",
    "Copy Collection": "Copiar coleção",
    "Save Collection": "Salvar coleção",
    "Build offline collector": "Criar Coletor Offline",
    "Notebooks": "Cadernos",
    "Full Screen": "Tela cheia",
    "Delete Notebook": "Excluir Caderno",
    "Notebook Uploads": "Carregamentos do Notebook",
    "Export Notebook": "Exportar Notebook",
    "FINISHED": "CONCLUÍDO",
    "RUNNING": "EXECUTAR",
    "STOPPED": "PARADO",
    "PAUSED": "PARADO",
    "ERROR": "Erro",
    "CANCELLED": "Cancelado",
    "Search for artifacts...": "Procurando por artefatos...",
    "Favorite Name": "Nome favorito",
    "Artifact": "Artefato",
    "No artifacts configured. Please add some artifacts to collect": "Nenhum artefato configurado. Por favor, adicione alguns artefatos para coletar",

    "Artifacts": "Artefatos",
    "Collected Artifacts": "Artefatos Coletados",
    "Flow ID": "ID do fluxo",
    "FlowId": "ID do fluxo",
    "Goto notebooks": "Ir para Cadernos",
    "Max Mb": "MaxMb",
    "Mb": "Mb",
    "Name": "nome",
    "Ops/Sec": "Operações/s",
    "Rows": "Linhas",
    "New Collection: Select Artifacts to collect":"Nova coleção: selecione artefatos para coletar",
    "Select Artifacts":"Selecionar artefatos",
    "Configure Parameters":"Configurar parâmetros",
    "Specify Resources":"Especificar recursos",
    "Review":"Revisão",
    "Launch":"Iniciar",
    "New Collection: Configure Parameters":"Nova coleção: configurar parâmetros",
    "New Collection: Specify Resources":"Nova coleção: especificar recursos",
    "New Collection: Review request":"Nova coleção: solicitação de revisão",
    "New Collection: Launch collection":"Nova coleção: iniciar coleção",

    "CPU Limit Percent":"Porcentagem de limite de CPU",
    "IOps/Sec":"IOps/s",
    "Max Execution Time in Seconds":"Tempo máx. de execução em segundos",
    "Max Idle Time in Seconds":"Tempo ocioso máximo em segundos",
    "If set collection will be terminated after this many seconds with no progress.":"Wenn die Satzsammlung nach so vielen Sekunden ohne Fortschritt beendet wird.",
    "Max Mb Uploaded":"Máximo de MB carregados",
    "Collection did not upload files":"A coleção não possui arquivos enviados",

    "Create Offline collector: Select artifacts to collect":"Criar coletor offline: selecione artefatos para coletar",
    "Configure Collection":"Configurar coleção",
    "Create Offline Collector: Configure artifact parameters":"Criar coletor offline: configurar parâmetros de artefato",
    "Create Offline Collector: Review request":"Criar coletor offline: Verificar solicitação",
    "Create Offline Collector: Create collector":"Criar coletor offline: Criar coletor",

    "Create Offline collector:  Configure Collector":"Criar coletor offline: configurar coletor",
    "Target Operating System":"SO de destino",
    "Password":"Senha",
    "Report Template":"Modelo de Relatório",
    "No Report":"Nenhum relatório",
    "Collection Type":"Tipo de coleção",
    "Zip Archive":"Arquivo zip",
    "Google Cloud Bucket":"Google Cloud Bucket",
    "AWS Bucket":"Balde da AWS",
    "SFTP Upload":"Carregamento SFTP",
    "Velociraptor Binary":"Velociraptor binário",
    "Temp directory":"Diretório temporário",
    "Temp location":"Localização temporária",
    "Compression Level":"Nível de compactação",
    "Output format":"Formato de saída",
    "CSV and JSON":"CSV e JSON",
    "Output Prefix":"Prefixo de saída",
    "Output filename prefix":"Imprimir prefixo do nome do arquivo",

    "DeleteHuntDialog": <>
        <p>Você está prestes a parar e excluir permanentemente todos os dados desta busca.</p>
        <p>Tem certeza de que deseja cancelar esta busca e excluir os dados coletados?</p>
    </>,

    "Started":"Iniciado",
    "Expires":"Expira",
    "Scheduled":"Planejado",
    "New Hunt":"Nova caça",
    "Run Hunt":"Executar caça",
    "Stop Hunt":"Parar caça",
    "Delete Hunt":"Excluir busca",
    "Copy Hunt":"Caça à cópia",
    "No hunts exist in the system. You can start a new hunt by clicking the New Hunt button above.":"Im System sind keine Jagden vorhanden. Sie können eine neue Jagd starten, indem Sie oben auf die Schaltfläche \"Neue Jagd\" klicken.",
    "Please select a hunt above":"Por favor, selecione uma busca acima",
    "Clients":"Clientes",
    "Notebook for Hunt": hunt_id=>"Caderno de Caça " + hunt_id,

    "Hunt ID":"ID da caça",
    "Creation Time":"Tempo de criação",
    "Expiry Time":"Tempo de validade",
    "Total scheduled":"Total planejado",
    "Finished clients":"Clientes prontos",
    "Full Download":"Baixar completo",
    "Summary Download":"Resumo de download",
    "Summary (CSV Only)":"Resumo (somente CSV)",
    "Summary (JSON Only)":"Resumo (somente JSON)",
    "name":"Nome",
    "size":"Tamanho",
    "date":"Data",
    "New Hunt - Configure Hunt":"Nova busca - Configurar busca",
    "Hunt description":"Descrição da caça",
    "Expiry":"Expiração",
    "Include Condition":"Incluir condição",
    "Run everywhere":"Corra em todos os lugares",
    "Exclude Condition":"Excluir condição",
    "Configure Hunt":"Configurar busca",
    "Estimated affected clients":"Estimativa de clientes afetados",
    "All Known Clients":"Todos os clientes conhecidos",
    "1 Day actives":"1 dia ativo",
    "1 Week actives":"1 semana ativa",
    "1 Month actives":"1 mês ativo",
    "Create Hunt: Select artifacts to collect":"Criar caça: selecionar artefatos para coletar",
    "Create Hunt: Configure artifact parameters":"Criar busca: configurar parâmetros de artefato",
    "Create Hunt: Specify resource limits":"Criar busca: especificar limites de recursos",
    "Create Hunt: Review request":"Criar busca: Verificar solicitação",
    "Create Hunt: Launch hunt":"Criar busca: Iniciar busca",

    "ClientId": "ID do cliente",
    "StartedTime":"Hora de início",
    "TotalBytes":"Total de Bytes",
    "TotalRows":"Total de linhas",

    "client_time":"Hora do cliente",
    "level":"Nível",
    "message":"Mensagem",

    "RecursiveVFSMessage": path=><>
      Você está prestes a buscar recursivamente todos os arquivos em <b>{path}</b>.
      <br/><br/>
      Isso permite que grandes quantidades de dados sejam transferidas do terminal. O limite de upload padrão é de 1 GB, mas você pode alterá-lo na tela Collected Artifacts.
      </>,

    "Textview":"Visualização de texto",
    "HexView":"HexView",
    "Refresh this directory (sync its listing with the client)":"Atualizar este diretório (sincronizar) com o cliente",
    "Recursively refresh this directory (sync its listing with the client)":"Dieses Verzeichnis rekursiv aktualisieren (seinen Eintrag mit dem Client synchronisieren)",
    "Recursively download this directory from the client":"Baixar este diretório recursivamente do cliente",
    "View Collection":"Ver coleção",
    "Size":"Tamanho",
    "Mode":"Modo",
    "mtime":"mtime",
    "atime":"uma vez",
    "ctime":"ctime",
    "btime":"btime",
    "Mtime":"Mtime",
    "Atime":"Tempo",
    "Ctime":"Chora",
    "Btime":"Btime",
    "Properties":"Propriedades",
    "No data available. Refresh directory from client by clicking above.":"Nenhum dado disponível. Atualize o diretório do cliente clicando acima.",
    "Please select a file or a folder to see its details here.":"Selecione um arquivo ou pasta aqui.",
    "Currently refreshing from the client":"Atualização atual do cliente",
    "Recursively download files":"Baixar arquivos recursivamente",

    "Home":"Em casa",
    "Hunt Manager":"Gerente de caça",
    "View Artifacts":"Mostrar artefatos",
    "Server Events":"Eventos do Servidor",
    "Server Artifacts":"Artefatos do Servidor",
    "Host Information":"Informações do host",
    "Virtual Filesystem":"Sistema de arquivos virtual",
    "Client Events":"Eventos do cliente",
    "This is a notebook for processing a hunt.":"Este é um notebook para lidar com uma caça.",
    "ToolLocalDesc":
        <>
        A ferramenta é fornecida pelo servidor Velociraptor
    Clientes, se aplicável. O cliente
    Armazene a ferramenta em cache em seu próprio disco e depois compare o hash
    tempo é necessário. As ferramentas só serão baixadas se
    Hash mudou.
        </>,
    "ServedFromURL": (base_path, url)=>
    <>
      Os clientes obtêm a ferramenta diretamente de
      <a href={base_path + url}>{url}</a> se
      necessário. Observe que, se o hash não corresponder ao
      hash esperado, os clientes rejeitarão o arquivo.
    </>,
    "ServedFromGithub": (github_project, github_asset_regex)=>
    <>
      O URL da ferramenta é atualizado por
      GitHub como a versão mais recente do projeto
      <b>{github_project}</b> que se encaixa
      <b>{github_asset_regex}</b>
    </>,
    "PlaceHolder":
        <>
        O hash da ferramenta é atualmente desconhecido. A ferramenta pela primeira vez
    for necessário, o Velociraptor irá baixá-lo de lá
    url upstream e calcule seu hash.
        </>,
    "ToolHash":
        <>
        O hash da ferramenta foi calculado. Se os clientes precisam usar
    Eles usam essa ferramenta para garantir que esse hash corresponda ao que estão fazendo
    Download.
        </>,
    "AdminOverride":
        <>
        A ferramenta foi carregada manualmente por um
    admin - não é atualizado automaticamente no
    próxima atualização do servidor Velociraptor.
        </>,
    "ToolError":
        <>
        O hash da ferramenta é desconhecido e não é um URL
    é definido. Será impossível usar esta ferramenta em um
    Artefato porque o Velociraptor não pode dissipá-lo. Você
    pode carregar um arquivo manualmente.
        </>,
    "OverrideToolDesc":
        <>
        Como administrador, você pode carregar manualmente um arquivo
    binário pode ser usado como esta ferramenta. Isso sobrescreve o
    configuração de url upstream e disponibilize sua ferramenta para todos
    Artefatos que precisam. Como alternativa, defina um URL para clientes
    para obter ferramentas.
        </>,

    "Include Labels":"Incluir marcadores",
    "Exclude Labels":"Excluir rótulos",
    "? for suggestions":"? para sugestões",
    "Served from URL":"Fornecido por URL",
    "Placeholder Definition":"Definição de espaço reservado",
    "Materialize Hash":"Materializar hash",
    "Tool":"Ferramenta",
    "Override Tool":"Ferramenta de substituição",
    "Select file":"Selecionar arquivo",
    "Click to upload file":"Clique para fazer upload do arquivo",
    "Set Serve URL":"Definir URL do servidor",
    "Served Locally":"Servir localmente",
    "Tool Hash Known":"Ferramenta hash conhecida",
    "Re-Download File":"Baixar arquivo novamente",
    'Recolha do cliente': "Recolha do cliente",
    'Recolher do cliente': 'Recolher do cliente',
    "Tool Name":"Nome da ferramenta",
    "Upstream URL":"URL ascendente",
    "Enpoint Filename":"enpoint filename",
    "Hash":"Hash",
    "Serve Locally":"Servir localmente",
    "Serve URL":"Fornecer URL",
    "Fetch from Client": "Obter do cliente",
    "Last Collected": "Escolhido por último",
    "Offset": "Deslocamento",
    "Show All": "Mostrar tudo",
    "Recent Hosts": "Hosts recentes",
    "Download JSON": "Baixar JSON",
    "Download CSV": "Baixar arquivo CSV",
    "Transform Table": "Tabela de Transformação",
    "Transformed": "Transformado",

    "Select a notebook from the list above.":"Selecione um bloco de anotações da lista acima.",
    "Cancel":"Cancelar",
    "Recalculate":"Recalcular",
    "Stop Calculating":"Finalizar cálculo",
    "Edit Cell":"Editar célula",
    "Up Cell":"Cell up",
    "Down Cell":"Célula abaixo",
    "Add Cell":"Adicionar célula",
    "Suggestion":"Sugestão",
    "Suggestions":"Sugestões",
    "Add Timeline":"Adicionar linha do tempo",
    "Add Cell From This Cell":"Adicionar célula desta célula",
    "Add Cell From Hunt":"Adicionar célula da caça",
    "Add Cell From Flow":"Adicionar célula do fluxo",
    "Rendered":"Renderizado",
    "Undo":"Desfazer",
    "Delete Cell":"Excluir célula",
    "Uptime":"Disponibilidade",
    "BootTime":"BootTime",
    "Procs":"Processos",
    "OS":"Sistema operacional",
    "Platform":"Plataforma",
    "PlatformFamily":"PlataformaFamília",
    "PlatformVersion":"Versão da plataforma",
    "KernelVersion":"Versão do Kernel",
    "VirtualizationSystem":"VirtualizationSystem",
    "VirtualizationRole":"Função de virtualização",
    "HostID":"HostID",
    "Exe":"Exe",
    "Fqdn":"Fqdn",
    "Create a new Notebook":"Criar um novo bloco de notas",
    "Collaborators":"Funcionário",
    "Submit":"Enviar",
    "Edit notebook ":"Editar caderno ",
    "Notebook uploads":"Carregamentos de notebooks",
    "User Settings":"Configurações do usuário",
    "Select a user": "Selecione um usuário",

    "Theme":"Tópico",
    "Select a theme":"Selecione um tópico",
    "Default Velociraptor":"Velociraptor padrão",
    "Velociraptor (light)":"Velociraptor (luz)",
    "Ncurses (light)":"Ncurses (luz)",
    "Velociraptor (dark)":"Velociraptor (escuro)",
    "Github dimmed (dark)":"Github esmaecido (escuro)",
    "Cool Gray (dark)":"Cinza frio (escuro)",
    "Strawberry Milkshake (light)":"Shake de morango (light)",
    "Downloads Password":"Baixar senha",
    "Default password to use for downloads":"Senha padrão para downloads",

    "Create Artifact from VQL":"Criar artefato do VQL",
    "Member":"Membro",
    "Response":"Resposta",
    "Super Timeline":"Grande linha do tempo",
    "Super-timeline name":"Nome da super linha do tempo",
    "Timeline Name":"Nome da coluna de tempo",
    "Child timeline name":"Nome da linha do tempo filha",
    "Time column":"Coluna Hora",
    "Time Column":"Coluna de tempo",
    "Language": "Idioma",
    "Match by label": "Corresponder por rótulo",
    "All known Clients": "Todos os clientes conhecidos",
    "X per second": x=><>{x} por segundo</>,
    "HumanizeDuration": difference=>{
        if (difference<0) {
            return <>
                Em {humanizeDuration(difference, {
                         round: true,
                         language: "pt",
                     })}
            </>;
        }
        return <>
            Antes de {humanizeDuration(difference, {
                         round: true,
                         language: "pt",
                     })}
        </>;
    },
    "Transform table": "Tabela de Transformação",
    "Sort Column": "Classificar Coluna",
    "Filter Regex": "Filtrar Regex",
    "Filter Column": "Coluna de filtro",
    "Select label to edit its event monitoring table": "Selecione o rótulo para editar sua tabela do monitor de eventos",
    "EventMonitoringCard":
        <>
        O monitoramento de eventos visa grupos de rótulos específicos.
        Selecione um grupo de rótulos acima para configurá-lo especificamente
    Artefatos de eventos direcionados a este grupo.
        </>,
    "Event Monitoring: Configure Label groups": "Monitor de eventos: configurar grupos de rótulos",
    "Configuring Label": "Configurar rótulo",
    "Event Monitoring Label Groups": "Grupos de rótulos do monitor de eventos",
    "Event Monitoring: Select artifacts to collect from label group ": "Monitoramento de eventos: selecione artefatos para coletar do grupo de rótulos ",
    "Artifact Collected": "Artefato coletado",
    "Event Monitoring: Configure artifact parameters for label group ":"Monitoramento de eventos: configurar parâmetros de artefato para o grupo de rótulos",
    "Event Monitoring: Review new event tables": "Monitor de eventos: verificar novas tabelas de eventos",

    "Server Event Monitoring: Select artifacts to collect on the server":"Serverereignisüberwachung: Wählen Sie Artefakte aus, die auf dem Server gesammelt werden sollen",
     "Server Event Monitoring: Configure artifact parameters for server":"Monitoramento de eventos do servidor: configurar parâmetros de artefato para o servidor",
    "Server Event Monitoring: Review new event tables":"Monitor de eventos do servidor: Verificar novas tabelas de eventos",
    "Configure Label Group":"Configurar grupo de rótulos",
    "Select artifact": "Selecionar Artefato",

    "Raw Data":"Dados brutos",
    "Logs":"Arquivo de registro",
    "Log":"Arquivo de registro",
    "Report":"Relatório",

    "NotebookId":"ID do Notebook",
    "Modified Time":"Hora alterada",
    "Time": "Hora",
    "No events": "Nenhum evento",
    "_ts": "Hora do Servidor",

    "Timestamp":"Carimbo de data e hora",
    "started":"Iniciado",
    "vfs_path":"Caminho do VFS",
    "file_size":"Tamanho do arquivo",
    "uploaded_size":"Tamanho enviado",
    "TablePagination": (from, to, size)=>
    <>Mostrar linha { from } a { to } de { size }</>,

    "Select a language":"Selecionar idioma",
    "English":"Inglês",
    "Deutsch":"Alemão",
    "Spanish": "Espanhol",
    "Portuguese": "Português",
    "French": "Francês",

    "Type":"Tipo",
    "Export notebooks":"Exportar Cadernos",
    "Export to HTML":"Exportar para HTML",
    "Export to Zip":"Exportar para zip",

    "Permanently delete Notebook":"Excluir bloco de anotações permanentemente",
    "You are about to permanently delete the notebook for this hunt":"Você está prestes a excluir permanentemente o notebook para esta caça",

    "Data":"Dados",
    "Served from GitHub":"Fornecido pelo GitHub",
    "Refresh Github":"Atualização do GitHub",
    "Github Project":"Projeto GitHub",
    "Github Asset Regex":"Github Asset Regex",
    "Admin Override":"Substituição de administrador",
    "Serve from upstream":"Servir de upstream",

    "Update server monitoring table":"Atualizar tabela de monitoramento do servidor",
    "Show server monitoring tables":"Ver tabelas de monitoramento do servidor",

    "Display timezone": "Mostrar fuso horário",
    "Select a timezone": "Selecione um fuso horário",

    "Update client monitoring table":"Atualizar tabela de monitoramento de clientes",
    "Show client monitoring tables":"Mostrar tabelas de monitoramento de clientes",
};

export default Portuguese;
