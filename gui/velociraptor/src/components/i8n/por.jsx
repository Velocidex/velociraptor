import _ from 'lodash';
import hex2a from "./utils";
import React from 'react';
import Alert from 'react-bootstrap/Alert';
import humanizeDuration from "humanize-duration";
import api from '../core/api-service.jsx';

import automated from "./por.json";


const Portuguese = {
    "SEARCH_CLIENTS": "Pesquisar clientes",
    "Quarantine description": (
        <>
          <p>Você está prestes a colocar este host em quarentena.</p>
          <p>
            Durante a quarentena, o host não pode
            comunicar-se com nenhuma outra rede, exceto com o
            Servidor Velociraptor.
          </p>
        </>),
    "Cannot Quarantine host": "Não foi possível colocar o host em quarentena",
    "Cannot Quarantine host message": (os_name, quarantine_artifact)=>
    <>
      <Alert variant="warning">
        { quarantine_artifact ?
          <p>Esta instância do Velociraptor não possui o artefato <b>{quarantine_artifact}</b> necessário para colocar em quarentena os hosts {os_name}.</p> :
          <p>Esta instância do Velociraptor não possui um nome de artefato definido para colocar em quarentena os hosts {os_name}.</p>
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
    "Operating System": "Sistema Operacional",
    "Hostname": "Nome do host",
    "FQDN": "FQDN",
    "Release": "Liberar",
    "Architecture": "Arquitetura",
    "Client Metadata": "Metadados do cliente",
    "Interrogate": "Consultar",
    "VFS": "VFS",
    "Collected": "Coletado",
    "Unquarantine Host": "Remover host da quarentena ",
    "Quarantine Host": "Colocar Host em quarentena",
    "Quarantine Message": "Mensagem de quarentena",
    "Add Label": "Adicionar rótulo",
    "Overview": "Visão geral",
    "VQL Drilldown": "Expandir VQL",
    "Shell": "Shell",
    "Close": "Fechar",
    "Connected": "Conectado",
    "seconds": "segundos",
    "minutes": "minutos",
    "hours": "horas",
    "days": "dias",
    "time_ago":  function(value, unit) {
        unit = Portuguese[unit] || unit;
        return 'Antes' + value + ' ' + unit;
    },
    "Online": "Online",
    "Label Clients": "Adicionar rótulo ao cliente",
    "Existing": "Existente",
    "A new label": "Um novo rótulo",
    "Add it!": "Adicione!",
    "Delete Clients": "Excluir clientes",
    "DeleteMessage": "Você está prestes a excluir permanentemente os seguintes clientes",
    "Yeah do it!": "Sim, faça isso!",
    "Goto Page": "Ir para a página",
    "Table is Empty": "A tabela está vazia",
    "OS Version": "Versão do Sistema Operacional",
    "Select a label": "Selecione um rótulo",
    "Expand": "Expandir",
    "Collapse": "Detalhamento",
    "Hide Output": "Ocultar Resultado",
    "Load Output": "Carregar Resultado",
    "Stop": "Parar",
    "Delete": "Excluir",
    "Run command on client": "Executar comando no cliente",
    "Type VQL to run on the client": "Digite a VQL a ser executada no cliente",
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
    "Hunt Artifact": "Artefato de Investigação",
    "Collect Artifact": "Coletar Artefato",
    "Upload Artifact Pack": "Carregar pacote de artefatos",
    "Search for artifact": "Encontrar artefato",
    "Search for an artifact to view it": "Procure um artefato para visualizá-lo",
    "Edit Artifact": nome=>{
        return "Editar artefato" + nome;
    },
    "Create a new artifact": "Criar um novo artefato",
    "Save": "Salvar",
    "Search": "Procurar",
    "Toggle Main Menu": "Alternar menu principal",
    "Main Menu": "Menu principal",
    "Welcome": "Bem-vindo",

    // Navegação do teclado.
    "Global hotkeys": "Teclas de atalho globais",
    "Goto dashboard": "Ir para o Painel",
    "Collected artifacts": "Artefatos Coletados",
    "Show/Hide keyboard hotkeys help": "Mostrar/ocultar ajuda para teclas de atalho do teclado",
    "Focus client search box": "Foco na caixa de pesquisa do cliente",
    "New Artifact Collection Wizard": "Assistente de nova coleta de artefatos",
    "Artifact Selection Step": "Etapa de seleção de artefato",
    "Parameters configuration Step": "Etapa de configuração de parâmetros",
    "Collection resource specification": "Capture Resource Specification",
    "Launch artifact": "Iniciar artefato",
    "Go to next step": "Ir para a próxima etapa",
    "Go to previous step": "Ir para a etapa anterior",
    "Select next collection": "Selecione a próxima coleta",
    "Select previous collection": "Selecionar coleta anterior",
    "View selected collection results": "Mostrar resultados da coleta selecionada",
    "View selected collection overview": "Mostrar visão geral da coleta selecionada",
    "View selected collection logs": "Ver logs da coleta selecionada",
    "View selected collection uploaded files": "Visualizar arquivos subidos da coleta selecionada",
    "Editor shortcuts": "Atalhos do Editor",
    "Popup the editor configuration dialog": "Abrir caixa de diálogo de configuração",
    "Save editor contents": "Salvar conteúdo do editor",
    "Keyboard shortcuts": "Atalhos de teclado",
    "Yes do it!": "Sim, faça isso!",
    "Inspect Raw JSON": "Verificar JSON bruto",
    "Raw Response JSON": "JSON de resposta não formatado",
    "Show/Hide Columns": "Mostrar/Ocultar Colunas",
    "Set All": "Definir tudo",
    "Clear All": "Excluir tudo",
    "Exit Fullscreen": "Sair da tela inteira",
    "Artifact Collection": "Coleta de Artefatos",
    "Uploaded Files": "Arquivos Subidos",
    "Results": "Resultados",
    "Flow Details": "Detalhes do fluxo",
    "Notebook for Collection": name=>"Notebook da coleta "+name,
    "Please click a collection in the above table":"Clique em uma coleta na tabela acima",
    "Artifact Names": "Nomes de Artefatos",
    "Creator": "Criador",
    "Create Time": "Hora de Criação",
    "Start Time": "Hora de Início",
    "Last Active": "Ativo pela última vez",
    "Duration": "Duração",
    " Running...": "Executando...",
    "State": "Estado",
    "Error": "Erro",
    "CPU Limit": "Limite de CPU",
    "IOPS Limit": "Limite de IOPS",
    "Timeout": "Tempo Limite",
    "Max Rows": "Máximo de Linhas",
    "Max MB": "Máx. MB",
    "Artifacts with Results": "Artefatos com Resultados",
    "Total Rows": "Total de Linhas",
    "Uploaded Bytes": "Bytes Enviados",
    "Files uploaded": "Arquivos Enviados",
    "Download Results": "Baixar Resultados",
    "Set a password in user preferences to lock the download file.": "Definir senha para proteger os arquivos baixados.",
    "Prepare Download": "Preparar download",
    "Prepare Collection Report": "Preparar relatório da coleta",
    "Available Downloads": "Downloads disponíveis",
    "Size (Mb)": "Tamanho (MB)",
    "Date": "Data",
    "Unlimited": "Ilimitado",
    "rows": "Linhas",
    "Request sent to client": "Solicitação enviada ao cliente",
    "Description": "Descrição",
    "Created": "Criado",
    "Manually add collection to hunt": "Adicionar coleta à Investigação",
    "No compatible hunts.": "Nenhuma Investigação compatível.",
    "Please create a hunt that collects one or more of the following artifacts.":"Favor criar uma Investigação que colete um ou mais dos seguintes artefatos.",
    "Requests": "Solicitações",
    "Notebook": "Notebook",
    "Permanently delete collection": "Excluir coleta permanentemente",
    "ArtifactDeletionDialog": (session_id, artifacts, total_bytes, total_rows)=>
    <>
      Você está prestes a excluir permanentemente a coleta de
      artefatos <b>{session_id}</b>.
      <br/>
      Esta coleta tinha os artefatos <b className="wrapped-text">
                    {artifacts}
      </b>
      <br/><br/>

      Calculado liberar { total_bytes.toFixed(0) } MB de armazenamento
      dados e { total_rows } linhas.
    </>,
    "Save this collection to your Favorites": "Salvar esta coleta em seus Favoritos",
    "ArtifactFavorites": artifacts=>
    <>
      Você pode facilmente reutilizar a mesma coleta salva em seus
      favoritos no futuro.
      <br/>
      Esta coleta continha os artefatos <b>{artifacts}</b>
      <br/><br/>
    </>,
    "New Favorite name": "Novo nome Favorito",
    "Describe this favorite": "Descreva este Favorito",
    "New Collection": "Nova coleta",
    "Add to hunt": "Adicionar à Investigação",
    "Delete Artifact Collection": "Excluir coleta de artefatos",
    "Cancel Artifact Collection": "Cancelar coleta de artefatos",
    "Copy Collection": "Copiar coleta",
    "Save Collection": "Salvar coleta",
    "Build offline collector": "Criar Coletor Offline",
    "Notebooks": "Notebooks",
    "Full Screen": "Tela Cheia",
    "Delete Notebook": "Excluir Notebook",
    "Notebook Uploads": "Uploads do Notebook",
    "Export Notebook": "Exportar Notebook",
    "FINISHED": "CONCLUÍDO",
    "RUNNING": "EXECUTAR",
    "STOPPED": "PARADO",
    "PAUSED": "PAUSADO",
    "ERROR": "ERRO",
    "CANCELLED": "CANCELLADO",
    "Search for artifacts...": "Procurando por artefatos...",
    "Favorite Name": "Nome Favorito",
    "Artifact": "Artefato",
    "No artifacts configured. Please add some artifacts to collect": "Nenhum artefato configurado. Por favor, adicione alguns artefatos para coletar",

    "Artifacts": "Artefatos",
    "Collected Artifacts": "Artefatos Coletados",
    "Flow ID": "ID do fluxo",
    "FlowId": "ID do fluxo",
    "Goto notebooks": "Ir para Notebooks",
    "Max Mb": "Max Mb",
    "Mb": "Mb",
    "Name": "Nome",
    "Ops/Sec": "Operações/s",
    "Rows": "Linhas",
    "New Collection: Select Artifacts to collect":"Nova Coleta: Selecione Artefatos para Coletar",
    "Select Artifacts":"Selecionar artefatos",
    "Configure Parameters":"Configurar Parâmetros",
    "Specify Resources":"Especificar Recursos",
    "Review":"Revisão",
    "Launch":"Iniciar",
    "New Collection: Configure Parameters":"Nova coleta: configurar parâmetros",
    "New Collection: Specify Resources":"Nova coleta: especificar recursos",
    "New Collection: Review request":"Nova coleta: solicitação de revisão",
    "New Collection: Launch collection":"Nova coleta: iniciar coleta",

    "CPU Limit Percent":"Porcentagem de Limite de CPU",
    "IOps/Sec":"IOps/s",
    "Max Execution Time in Seconds":"Tempo Máx. de Execução em Segundos",
    "Max Idle Time in Seconds":"Tempo Ocioso Máximo em Segundos",
    "If set collection will be terminated after this many seconds with no progress.":"Caso configurado, a coleta terminará após se passar o numero selecionado em segundos sem nenhum progresso.",
    "Max bytes Uploaded":"Máximo de MB carregados",
    "Collection did not upload files":"A coleta não possui arquivos enviados",

    "Create Offline collector: Select artifacts to collect":"Criar Coletor Offline: Selecione Artefatos para Coletar",
    "Configure Collection":"Configurar Coleta",
    "Create Offline Collector: Configure artifact parameters":"Criar Coletor Offline: Configurar Parâmetros de Artefato",
    "Create Offline Collector: Review request":"Criar Coletor Offline: Verificar Solicitação",
    "Create Offline Collector: Create collector":"Criar Coletor Offline: Criar Coletor",

    "Create Offline collector:  Configure Collector":"Criar Coletor Offline: Configurar Coletor",
    "Target Operating System":"Sistema Operational Alvo",
    "Password":"Senha",
    "Report Template":"Modelo de Relatório",
    "No Report":"Nenhum Relatório",
    "Collection Type":"Tipo de Coleta",
    "Zip Archive":"Arquivo Zip",
    "Google Cloud Bucket":"Google Cloud Bucket",
    "AWS Bucket":"Balde da AWS",
    "SFTP Upload":"Carregamento SFTP",
    "Velociraptor Binary":"Binário do Velociraptor",
    "Temp directory":"Diretório temporário",
    "Temp location":"Localização temporária",
    "Compression Level":"Nível de compactação",
    "Output format":"Formato de saída",
    "CSV and JSON":"CSV e JSON",
    "Output Prefix":"Prefixo do saída",
    "Output filename prefix":"Adicionar prefixo no nome do arquivo de saída",

    "DeleteHuntDialog": <>
        <p>Você está prestes a parar e excluir permanentemente todos os dados desta busca.</p>
        <p>Tem certeza de que deseja cancelar esta busca e excluir os dados coletados?</p>
    </>,

    "Started":"Iniciado",
    "Expires":"Expira",
    "Scheduled":"Planejado",
    "New Hunt":"Nova Investigação",
    "Run Hunt":"Executar Investigação",
    "Stop Hunt":"Parar Investigação",
    "Delete Hunt":"Excluir Investigação",
    "Copy Hunt":"Copiar a Investigação",
    "No hunts exist in the system. You can start a new hunt by clicking the New Hunt button above.":"Nenhuma Investigação existe no sistema. Você pode iniciar uma Investigação clicando no botão Nova Investigação acima",
    "Please select a hunt above":"Por favor, selecione uma Investigação acima",
    "Clients":"Clientes",
    "Notebook for Hunt": hunt_id=>"Notebook de Investigação " + hunt_id,

    "Hunt ID":"ID da Investigação",
    "Creation Time":"Data de Criação",
    "Expiry Time":"Data de Validade",
    "Total scheduled":"Total planejado",
    "Finished clients":"Clientes prontos",
    "Full Download":"Download Completo",
    "Summary Download":"Download do Resumo",
    "Summary (CSV Only)":"Resumo (somente CSV)",
    "Summary (JSON Only)":"Resumo (somente JSON)",
    "name":"nome",
    "size":"tamanho",
    "date":"data",
    "New Hunt - Configure Hunt":"Nova Investigação - Configurar Investigação",
    "Hunt description":"Descrição da Investigação",
    "Expiry":"Expiração",
    "Include Condition":"Incluir Condição",
    "Run everywhere":"Executar em tudo",
    "Exclude Condition":"Condição de Exclusão",
    "Configure Hunt":"Configurar Investigação",
    "Estimated affected clients":"Estimativa de clientes afetados",
    "All Known Clients":"Todos os Clientes Conhecidos",
    "1 Day actives":"Ativos à 1 Dia",
    "1 Week actives":"Ativos à 1 Semana",
    "1 Month actives":"Ativos à 1 Mês",
    "Create Hunt: Select artifacts to collect":"Criar Investigação: Selecionar Artefatos para Coletar",
    "Create Hunt: Configure artifact parameters":"Criar Investigação: Configurar Parâmetros de Artefato",
    "Create Hunt: Specify resource limits":"Criar Investigação: Especificar Limites de Recursos",
    "Create Hunt: Review request":"Criar Investigação: Verificar Solicitação",
    "Create Hunt: Launch hunt":"Criar Investigação: Iniciar Investigação",

    "ClientId": "ClientId",
    "StartedTime":"DataInício",
    "TotalBytes":"TotalBytes",
    "TotalRows":"TotalLinhas",

    "client_time":"hora_cliente",
    "level":"nível",
    "message":"mensagem",

    "RecursiveVFSMessage": path=><>
      Você está prestes a buscar recursivamente todos os arquivos em <b>{path}</b>.
      <br/><br/>
      Isso permite que grandes quantidades de dados sejam transferidas do terminal. O limite de upload padrão é de 1 GB, mas você pode alterá-lo na tela Artefatos Colectados.
      </>,

    "Textview":"VerTexto",
    "HexView":"VerHex",
    "Refresh this directory (sync its listing with the client)":"Atualizar este diretório (sincronizar lista com o cliente)",
    "Recursively refresh this directory (sync its listing with the client)":"Atualizar recursivamente este diretório (sincronizar lista com o cliente)",
    "Recursively download this directory from the client":"Baixar este diretório recursivamente do cliente",
    "View Collection":"Ver coleta",
    "Size":"Tamanho",
    "Mode":"Modo",
    "mtime":"mtime",
    "atime":"atime",
    "ctime":"ctime",
    "btime":"btime",
    "Mtime":"Mtime",
    "Atime":"Atime",
    "Ctime":"Ctime",
    "Btime":"Btime",
    "Properties":"Propriedades",
    "No data available. Refresh directory from client by clicking above.":"Nenhum dado disponível. Atualize o diretório do cliente clicando acima.",
    "Please select a file or a folder to see its details here.":"Selecione um arquivo ou pasta para ver seus detalhes aqui.",
    "Currently refreshing from the client":"Atualizando pelo cliente",
    "Recursively download files":"Baixar arquivos recursivamente",

    "Home":"Home",
    "Hunt Manager":"Gerente de Investigações",
    "View Artifacts":"Ver Artefatos",
    "Server Events":"Eventos do Servidor",
    "Server Artifacts":"Artefatos do Servidor",
    "Host Information":"Informações do Host",
    "Virtual Filesystem":"Sistema de Arquivos Virtual",
    "Client Events":"Eventos do Cliente",
    "This is a notebook for processing a hunt.":"Este é um notebook para lidar com uma investigação.",
    "ToolLocalDesc":
        <>
        A ferramenta é fornecida pelo Servidor Velociraptor,
    se aplicável. O cliente armazena a ferramenta em cache
    em seu próprio disco e depois compara seu hash quando necessário.
    As ferramentas só serão baixadas se o hash mudar.
        </>,
    "ServedFromURL": (url)=>
    <>
      Os clientes obtêm a ferramenta diretamente
      de <a href={api.href(url)}>{url}</a> se
      necessário. Observe que, se o hash não corresponder ao
      hash esperado, os clientes rejeitarão o arquivo.
    </>,
    "ServedFromGithub": (github_project, github_asset_regex)=>
    <>
      O URL da ferramenta é atualizado por
      GitHub como a versão mais recente do
      projeto <b>{github_project}</b> que se
      encaixa <b>{github_asset_regex}</b>
    </>,
    "PlaceHolder":
        <>
        O hash da ferramenta é atualmente desconhecido. Na primeira vez que a
    ferramenta for requisitada, o Velociraptor irá baixá-la da URL indicada
    e calculará seu hash.
        </>,
    "ToolHash":
        <>
        O hash da ferramenta foi calculado. Se os clientes precisarem usar,
    eles usarão este hash para garantir que a ferramenta corresponde àquela que estão fazendo
    Download.
        </>,
    "AdminOverride":
        <>
        A ferramenta foi carregada manualmente por um
    Administrador - não é atualizada automaticamente no
    próxima atualização do Servidor Velociraptor.
        </>,
    "ToolError":
        <>
        O hash da ferramenta é desconhecido e não há um URL
    definido. Será impossível usar esta ferramenta em um
    Artefato porque o Velociraptor não poderá materializá-la. Você
    pode carregar um arquivo manualmente.
        </>,
    "OverrideToolDesc":
        <>
        Como administrador, você pode carregar manualmente um arquivo
    binário para utilizá-lo como ferramenta. Isso sobrescreve o
    configuração de URL upstream e disponibiliza sua ferramenta para todos
    Artefatos que precisam. Como alternativa, defina uma URL conhecida para
    clientes obterem ferramentas.
        </>,

    "Include Labels":"Incluir Rótulos",
    "Exclude Labels":"Excluir Rótulos",
    "? for suggestions":"? para sugestões",
    "Served from URL":"Fornecido por URL",
    "Placeholder Definition":"Definição de Espaço Reservado",
    "Materialize Hash":"Materializar Hash",
    "Tool":"Ferramenta",
    "Override Tool":"Sobrescrever Ferramenta",
    "Select file":"Selecionar arquivo",
    "Click to upload file":"Clique para fazer upload do arquivo",
    "Set Serve URL":"Definir URL para Servir",
    "Served Locally":"Servir Localmente",
    "Tool Hash Known":"Hash Conhecido de Ferramenta",
    "Re-Download File":"Baixar Arquivo Novamente",
    'Recolha do cliente': "Baixar do Cliente",
    'Recolher do cliente': 'Baixar Novamente do Cliente',
    "Tool Name":"Nome da Ferramenta",
    "Upstream URL":"URL Upstream",
    "Endpoint Filename":"Nome do Arquivo no Host",
    "Hash":"Hash",
    "Serve Locally":"Servir Localmente",
    "Serve URL":"URL Servindo",
    "Fetch from Client": "Obter do Cliente",
    "Last Collected": "Coletado por Último",
    "Offset": "Offset",
    "Show All": "Mostrar Tudo",
    "Recent Hosts": "Hosts Recentes",
    "Download JSON": "Baixar JSON",
    "Download CSV": "Baixar CSV",
    "Transform Table": "Tabela de Transformação",
    "Transformed": "Transformado",

    "Select a notebook from the list above.":"Selecione um notebook da lista acima.",
    "Cancel":"Cancelar",
    "Recalculate":"Recalcular",
    "Stop Calculating":"Finalizar Cálculo",
    "Edit Cell":"Editar Célula",
    "Up Cell":"Mover Célula Acima",
    "Down Cell":"Mover Célula Abaixo",
    "Add Cell":"Adicionar Célula",
    "Suggestion":"Sugestão",
    "Suggestions":"Sugestões",
    "Add Timeline":"Adicionar Timeline",
    "Add Cell From This Cell":"Adicionar Célula desta Célula",
    "Add Cell From Hunt":"Adicionar Célula da Investigação",
    "Add Cell From Flow":"Adicionar Célula do Fluxo",
    "Rendered":"Renderizado",
    "Undo":"Desfazer",
    "Delete Cell":"Excluir Célula",
    "Uptime":"Disponibilidade",
    "BootTime":"BootTime",
    "Procs":"Processos",
    "OS":"Sistema Operacional",
    "Platform":"Plataforma",
    "PlatformFamily":"FamíliaPlataforma",
    "PlatformVersion":"VersãoPlataforma",
    "KernelVersion":"VersãoKernel",
    "VirtualizationSystem":"SistemaVirtualização",
    "VirtualizationRole":"FunçãoVirtualização",
    "HostID":"HostID",
    "Exe":"Exe",
    "Fqdn":"Fqdn",
    "Create a new Notebook":"Criar um novo Notebook",
    "Collaborators":"Colaboradores",
    "Submit":"Enviar",
    "Edit notebook ":"Editar Notebook ",
    "Notebook uploads":"Carregamentos de Notebooks",
    "User Settings":"Configurações do Usuário",
    "Select a user": "Selecione um Usuário",

    "Theme":"Tópico",
    "Select a theme":"Selecione um tópico",
    "Default Velociraptor":"Velociraptor Padrão",
    "Velociraptor Classic (light)": "Velociraptor Clássico (leve)",
    "Velociraptor (light)":"Velociraptor (leve)",
    "Ncurses (light)":"Ncurses (leve)",
    "Ncurses (dark)":"Ncurses (escuro)",
    "Velociraptor (dark)":"Velociraptor (escuro)",
    "Github dimmed (dark)":"Github esmaecido (escuro)",
    "Github (light)":"Github (leve)",
    "Cool Gray (dark)":"Cinza frio (escuro)",
    "Strawberry Milkshake (light)":"Strawberry Milkshake (leve)",
    "Downloads Password":"Senha para Downloads",
    "Default password to use for downloads":"Senha padrão para downloads",

    "Create Artifact from VQL":"Criar artefato do VQL",
    "Member":"Membro",
    "Response":"Resposta",
    "Super Timeline":"Super Timeline",
    "Super-timeline name":"Nome da Super-timeline",
    "Timeline Name":"Nome da Timeline",
    "Child timeline name":"Nome da timeline filha",
    "Time column":"Coluna Hora",
    "Time Column":"Coluna hora",
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
    "Sort Column": "Ordenar Coluna",
    "Filter Regex": "Filtrar Regex",
    "Filter Column": "Filtrar Coluna",
    "Select label to edit its event monitoring table": "Selecione o rótulo para editar sua tabela de monitoramento de eventos",
    "EventMonitoringCard":
        <>
        O monitoramento de eventos visa grupos de rótulos específicos.
        Selecione um grupo de rótulos acima para configurar especificamente
    Artefatos de eventos direcionados a este grupo.
        </>,
    "Event Monitoring: Configure Label groups": "Monitoramento de Eventos: Configurar Grupos de Rótulos",
    "Configuring Label": "Configurar Rótulo",
    "Event Monitoring Label Groups": "Grupos de Rótulos de Monitoramento de Eventos",
    "Event Monitoring: Select artifacts to collect from label group ": "Monitoramento de eventos: Selecione artefatos para coletar do grupo de rótulos ",
    "Artifact Collected": "Artefato coletado",
    "Event Monitoring: Configure artifact parameters for label group ":"Monitoramento de eventos: configurar parâmetros de artefato para o grupo de rótulos",
    "Event Monitoring: Review new event tables": "Monitor de eventos: verificar novas tabelas de eventos",

    "Server Event Monitoring: Select artifacts to collect on the server":"Monitoramento de Eventos do Servidor: Selecionar artefatos para colectar no servidor.",
     "Server Event Monitoring: Configure artifact parameters for server":"Monitoramento de Eventos do Servidor: Configurar parâmetros de artefato para o servidor",
    "Server Event Monitoring: Review new event tables":"Monitoramento de Eventos do Servidor: Revisar novas tabelas de eventos",
    "Configure Label Group":"Configurar Grupo de Rótulos",
    "Select artifact": "Selecionar Artefato",

    "Raw Data":"Dados Brutos",
    "Logs":"Logs",
    "Log":"Log",
    "Report":"Relatório",

    "NotebookId":"NotebookId",
    "Modified Time":"Hora Alterada",
    "Time": "Hora",
    "No events": "Nenhum evento",
    "_ts": "Hora do Servidor",

    "Timestamp":"Timestamp",
    "started":"iniciado",
    "vfs_path":"vfs_path",
    "file_size":"file_size",
    "uploaded_size":"uploaded_size",

    "Select a language":"Selecionar idioma",
    "English":"Inglês",
    "Deutsch":"Alemão",
    "Spanish": "Espanhol",
    "Portuguese": "Português",
    "French": "Francês",
    "Japanese": "Japonês",

    "Type":"Tipo",
    "Export notebooks":"Exportar notebooks",
    "Export to HTML":"Exportar para HTML",
    "Export to Zip":"Exportar para Zip",

    "Permanently delete Notebook":"Excluir Notebook permanentemente",
    "You are about to permanently delete the notebook for this hunt":"Você está prestes a excluir permanentemente o Notebook para esta investigação",

    "Data":"Dados",
    "Served from GitHub":"Fornecido pelo GitHub",
    "Refresh Github":"Atualizar GitHub",
    "Github Project":"Projeto GitHub",
    "Github Asset Regex":"Github Asset Regex",
    "Admin Override":"Admin Override",
    "Serve from upstream":"Servir de upstream",

    "Update server monitoring table":"Atualizar tabela de monitoramento do servidor",
    "Show server monitoring tables":"Ver tabelas de monitoramento do servidor",

    "Display timezone": "Mostrar fuso horário",
    "Select a timezone": "Selecione um fuso horário",

    "Update client monitoring table":"Atualizar tabela de monitoramento de clientes",
    "Show client monitoring tables":"Mostrar tabelas de monitoramento de clientes",
    "Urgent": "Urgente",
    "Skip queues and run query urgently": "Pule filas e execute a consulta com urgência",

    // Below need verification
    "Role_administrator": "Administrador do servidor",
    "Role_org_admin": "Administrador da organização",
    "Role_reader": "Usuário somente leitura",
    "Role_analyst": "Analista",
    "Role_investigator" : "Investigador",
    "Role_artifact_writer": "Escritor de artefato",
     "Role_api": "Cliente de API somente leitura",

     "Perm_ANY_QUERY": "Qualquer consulta",
     "Perm_PUBISH": "Publicar",
     "Perm_READ_RESULTS": "Ler resultados",
     "Perm_LABEL_CLIENT": "Clientes de etiquetas",
     "Perm_COLLECT_CLIENT": "Coletar cliente",
     "Perm_START_HUNT": "Iniciar busca",
     "Perm_COLLECT_SERVER": "Coletar servidor",
     "Perm_ARTIFACT_WRITER": "Gravador de artefato",
     "Perm_SERVER_ARTIFACT_WRITER": "Gravador de artefato do servidor",
     "Perm_EXECVE" : "EXECVE",
     "Perm_NOTEBOOK_EDITOR": "Editor de Caderno",
     "Perm_SERVER_ADMIN": "Administrador do servidor",
     "Perm_ORG_ADMIN": "Administrador da organização",
     "Perm_IMPERSONATION": "Impersonation",
     "Perm_FILESYSTEM_READ": "Leitura do sistema de arquivos",
     "Perm_FILESYSTEM_WRITE": "Gravação do sistema de arquivos",
     "Perm_MACHINE_STATE": "Estado da máquina",
     "Perm_PREPARE_RESULTS": "Preparar resultados",
     "Perm_DATASTORE_ACCESS": "Acesso ao armazenamento de dados",

     "ToolPerm_ANY_QUERY": "Emitir qualquer consulta ",
     "ToolPerm_PUBISH": "Publicar eventos em filas do lado do servidor (normalmente não é necessário)",
     "ToolPerm_READ_RESULTS": "Ler resultados de buscas, fluxos ou notebooks já executados",
     "ToolPerm_LABEL_CLIENT": "Pode manipular rótulos e metadados do cliente",
     "ToolPerm_COLLECT_CLIENT": "Agendar ou cancelar novas coletas em clientes",
     "ToolPerm_START_HUNT": "Iniciar uma nova busca",
     "ToolPerm_COLLECT_SERVER": "Agendar novas coletas de artefatos nos servidores Velociraptor",
     "ToolPerm_ARTIFACT_WRITER": "Adicionar ou editar artefatos personalizados executados no servidor",
     "ToolPerm_SERVER_ARTIFACT_WRITER": "Adicionar ou editar artefatos personalizados executados no servidor",
     "ToolPerm_EXECVE": "Permitido executar comandos arbitrários em clientes",
     "ToolPerm_NOTEBOOK_EDITOR": "Permitido alterar blocos de anotações e células",
     "ToolPerm_SERVER_ADMIN": "Permitido para gerenciar a configuração do servidor",
     "ToolPerm_ORG_ADMIN": "Permitido para gerenciar organizações",
     "ToolPerm_IMPERSONATION": "Permite que o usuário especifique um nome de usuário diferente para o plug-in query()",
     "ToolPerm_FILESYSTEM_READ": "Permitido ler arquivos arbitrários do sistema de arquivos",
     "ToolPerm_FILESYSTEM_WRITE": "Permitido criar arquivos no sistema de arquivos",
     "ToolPerm_MACHINE_STATE": "Permitido coletar informações de estado de máquinas (por exemplo, pslist())",
     "ToolPerm_PREPARE_RESULTS": "Permitido criar arquivos zip",
     "ToolPerm_DATASTORE_ACCESS": "Acesso permitido ao armazenamento de dados brutos",
};

_.each(automated, (v, k)=>{
    Portuguese[hex2a(k)] = v;
});

export default Portuguese;
