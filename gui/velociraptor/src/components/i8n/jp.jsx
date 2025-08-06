import _ from 'lodash';
import hex2a from "./utils";
import api from '../core/api-service.jsx';

import React from 'react';
import Alert from 'react-bootstrap/Alert';
import humanizeDuration from "humanize-duration";

import automated from "./jp.json";

const Japanese = {
    "SEARCH_CLIENTS": "クライアント検索",
    "Quarantine description": (<>
          <p>このホストを隔離しようとしています。</p>
          <p>
            隔離中は、Velociraptorサーバを除く他のネットワークと通信することはできません。
          </p>
        </>),
    "Cannot Quarantine host": "ホストを隔離できない",
    "Cannot Quarantine host message": (os_name, quarantine_artifact)=>
        <>
          <Alert variant="warning">
            { quarantine_artifact ?
              <p>このVelociraptorインスタンスは{os_name}を実行しているホストを検疫するために必要な<b>{quarantine_artifact}</b>アーティファクトを持っていません。</p> :
              <p>このVelociraptorインスタンスには、{os_name}を実行しているホストを検疫するためのアーティファクト名が定義されていません。</p>
            }
          </Alert>
        </>,
    "Client ID": "クライアントID",
    "Agent Version": "エージェントバージョン",
    "Agent Name": "エージェント名",
    "First Seen At": "最初接続日時",
    "Last Seen At": "最新接続日時",
    "Last Seen IP": "最新IPアドレス",
    "Labels": "ラベル",
    "Operating System": "OS",
    "Hostname": "ホスト名",
    "FQDN": "FQDN",
    "Release": "リリース",
    "Architecture": "アーキテクチャ",
    "Client Metadata": "クライアントメタデータ",
    "Interrogate": "調査する",
    "VFS": "VFS",
    "Collected": "収集した",
    "Unquarantine Host": "隔離されていないホスト",
    "Quarantine Host": "隔離されたホスト",
    "Quarantine Message": "隔離メッセージ",
    "Add Label": "ラベルの追加",
    "Overview": "概要",
    "VQL Drilldown": "VQLの詳細調査",
    "Shell": "シェル",
    "Close": "閉じる",
    "Connected": "接続された",
    "seconds": "秒",
    "minutes": "分",
    "hours": "時",
    "days": "日",
    "time_ago": function(value, unit) {
        unit = Japanese[unit] || unit;
        return value + ' ' + unit + ' 前';
    },
    "Online": "オンライン",
    "Label Clients": "クライアントのラベル付け",
    "Existing": "既存",
    "A new label": "新規ラベル",
    "Add it!": "追加する！",
    "Delete Clients": "クライアントの削除",
    "DeleteMessage": "以下のクライアントを永久に削除しようとしています。",
    "Yeah do it!": "そうだ、やってくれ！",
    "Goto Page": "ページを開く",
    "Table is Empty": "テーブルが空",
    "OS Version": "OSバージョン",
    "Select a label": "ラベルの選択",
    "Expand": "拡大する",
    "Collapse": "閉じる",
    "Hide Output": "出力の非表示",
    "Load Output": "出力をロードする",
    "Stop": "停止",
    "Delete": "削除",
    "Run command on client": "クライアントでコマンドを実行する",
    "Type VQL to run on the client": "VQLを入力して、クライアントで実行する",
    "Run VQL on client": "VQLをクライアントで実行する",
    "Artifact details": "アーティファクトの詳細",
    "Artifact Name": "アーティファクト名",
    "Upload artifacts from a Zip pack": "アーティファクトのZipパックをアップロードする",
    "Select artifact pack (Zip file with YAML definitions)": "アーティファクトパック（YAML定義のZipファイル）の選択",
    "Click to upload artifact pack file": "クリックすると、アーティファクトパックファイルがアップロードされる",
    "Delete an artifact": "アーティファクトの削除",
    "You are about to delete": name=>name+"が削除されます。",
    "Add an Artifact": "Agregar un artefacto",
    "Edit an Artifact": "アーティファクトの編集",
    "Delete Artifact": "アーティファクトの削除",
    "Hunt Artifact": "アーティファクトのハント",
    "Collect Artifact": "アーティファクトの収集",
    "Upload Artifact Pack": "アーティファクトパックのアップロード",
    "Search for artifact": "アーティファクトの検索",
    "Search for an artifact to view it": "アーティファクトを検索して表示する",
    "Edit Artifact": name=>{
      return name + "アーティファクトを編集する。";
    },
    "Create a new artifact": "新規アーティファクトを作成する",
    "Save": "保存",
    "Search": "検索",
    "Toggle Main Menu": "メインメニューの切り替え",
    "Main Menu": "メインメニュー",
    "Welcome": "いらっしゃいませ",

    // Keyboard navigation.
    "Global hotkeys": "グローバルホットキー",
    "Goto dashboard": "ダッシュボードを開く",
    "Collected artifacts": "収集されたアーティファクト",
    "Show/Hide keyboard hotkeys help": "キーボードホットキーヘルプの表示/非表示",
    "Focus client search box": "クライアント検索ボックス",
    "New Artifact Collection Wizard": "新規アーティファクトコレクションウィザード",
    "Artifact Selection Step": "アーティファクト選択のステップ",
    "Parameters configuration Step": "パラメータ設定のステップ",
    "Collection resource specification": "コレクションリソースの仕様",
    "Launch artifact": "アーティファクトを調べる",
    "Go to next step": "次のステップへ進む",
    "Go to previous step": "前のステップへ戻る",
    "Select next collection": "次のコレクションを選択する",
    "Select previous collection": "前のコレクションを選択する",
    "View selected collection results": "選択したコレクションの結果を表示する",
    "View selected collection overview": "選択したコレクションの概要を表示する",
    "View selected collection logs": "選択したコレクションログを表示する",
    "View selected collection uploaded files": "選択したコレクションのアップロードされたファイルを表示する",
    "Editor shortcuts": "エディタのショートカット",
    "Popup the editor configuration dialog": "エディタ設定ダイアログボックスのポップアップ",
    "Save editor contents": "エディタの内容を保存する",
    "Keyboard shortcuts": "キーボードショートカット",
    "Yes do it!": "はい！",
    "Inspect Raw JSON": "生のJSONデータを確認する",
    "Raw Response JSON": "生のJSONレスポンス",
    "Show/Hide Columns": "カラムの表示/非表示",
    "Set All": "すべてを設定する",
    "Clear All": "すべてをクリアする",
    "Exit Fullscreen": "フルスクリーンを閉じる",
    "Artifact Collection": "アーティファクトコレクション",
    "Uploaded Files": "アップロードされたファイル",
    "Results": "結果",
    "Flow Details": "フローの詳細",
    "Notebook for Collection": name=>name + "コレクションのノートブック",
    "Please click a collection in the above table":"上のテーブルからコレクションをクリックしてください",
    "Artifact Names": "アーティファクト名",
    "Creator": "作者",
    "Create Time": "作成時間",
    "Start Time": "開始時間",
    "Last Active": "最終稼働時間",
    "Duration": "継続時間",
    " Running...": " 実行中...",
    "State": "状態",
    "Error": "エラー",
    "CPU Limit": "CPUリミット",
    "IOPS Limit": "IOPSリミット",
    "Timeout": "タイムアウト",
    "Max Rows": "最大列数",
    "Max MB": "最大MB数",
    "Artifacts with Results": "結果のあるアーティファクト",
    "Total Rows": "合計行数",
    "Uploaded Bytes": "アップロードされたバイト数",
    "Files uploaded": "アップロードされたファイル",
    "Download Results": "結果をダウンロードする",
    "Set a password in user preferences to lock the download file.": "ユーザ設定でパスワードを設定し、ダウンロードファイルをロックする。",
    "Prepare Download": "ダウンロードの準備",
    "Prepare Collection Report": "コレクションレポートの準備",
    "Available Downloads": "可能なダウンロード",
    "Size (Mb)": "サイズ (MB)",
    "Date": "日付",
    "Unlimited": "無制限",
    "rows": "行数",
    "Request sent to client": "リクエストがクライアントに送信された",
    "Description": "詳細",
    "Created": "作成された",
    "Manually add collection to hunt": "手動でハントにコレクションを追加する",
    "No compatible hunts.": "互換性のあるハントはありません。",
    "Please create a hunt that collects one or more of the following artifacts.":"以下のアーティファクトを1つ以上収集するハントを作ってください。",
    "Requests": "リクエスト",
    "Notebook": "ノートブック",
    "Permanently delete collection": "コレクションを永久に削除する",
    "ArtifactDeletionDialog": (session_id, artifacts, total_bytes, total_rows)=>
    <>
      アーティファクトコレクションを永久に削除しようとしています。<b>{session_id}</b>.
      <br/>
      コレクションのアーティファクト： <b className="wrapped-text">
         {artifacts}
      </b>
      <br/><br/>

      { total_bytes.toFixed(0) } Mbのデータと{ total_rows }行が削除されます。
    </>,
    "Save this collection to your Favorites": "このコレクションをお気に入りに保存する",
    "ArtifactFavorites": artifacts=>
    <>
      今後、お気に入りから同じコレクションを簡単に集めることができます。
      <br/>
      このコレクションのアーティファクト: <b>{artifacts}</b>
      <br/><br/>
    </>,
    "New Favorite name": "新規お気に入りの名前",
    "Describe this favorite": "お気に入りを設定する",
    "New Collection": "収集する",
    "Add to hunt": "ハントに追加する",
    "Delete Artifact Collection": "アーティファクトコレクションを削除する",
    "Cancel Artifact Collection": "アーティファクトコレクションをキャンセルする",
    "Copy Collection": "コレクションをコピーする",
    "Save Collection": "コレクションを保存する",
    "Build offline collector": "オフラインコレクタの作成",
    "Notebooks": "ノートブック",
    "Full Screen": "フルスクリーン",
    "Delete Notebook": "ノートブックの削除",
    "Notebook Uploads": "ノートブックのアップロード",
    "Export Notebook": "ノートブックのエクスポート",
    "FINISHED": "終了",
    "RUNNING": "実行中",
    "STOPPED": "停止",
    "PAUSED": "一時停止",
    "ERROR": "エラー",
    "CANCELLED": "キャンセルされた",
    "Search for artifacts...": "アーティファクトの検索...",
    "Favorite Name": "お気に入りの名前",
    "Artifact": "アーティファクト",
    "No artifacts configured. Please add some artifacts to collect": "アーティファクトは設定されていません。収集するアーティファクトを追加してください。",

    "Artifacts": "アーティファクト",
    "Collected Artifacts": "収集されたアーティファクト",
    "Flow ID": "フローID",
    "FlowId": "フローID",
    "Goto notebooks": "ノートブックを開く",
    "Max Mb": "最大Mb",
    "Mb": "Mb",
    "Name": "名前",
    "Ops/Sec": "Ops/Sec",
    "Rows": "行数",
    "New Collection: Select Artifacts to collect":"新規コレクション: 収集するアーティファクトを選択する",
    "Select Artifacts":"アーティファクトを選択する",
    "Configure Parameters":"パラメータを設定する",
    "Specify Resources":"リソースを指定する",
    "Review":"レビュー",
    "Launch":"起動",
    "New Collection: Configure Parameters":"新規コレクション: パラメータを設定する",
    "New Collection: Specify Resources":"新規コレクション: リソースを指定する",
    "New Collection: Review request":"新規コレクション: レビューをリクエストする",
    "New Collection: Launch collection":"新規コレクション: 収集する",

    "CPU Limit Percent":"CPUリミット率",
    "IOps/Sec":"IOps/Sec",
    "Max Execution Time in Seconds":"最大実行時間(秒)",
    "Max Idle Time in Seconds":"最大アイドル時間(秒)",
    "If set collection will be terminated after this many seconds with no progress.":"設定された場合、収集はこの秒数後に何も進行していない状態で終了されます。",
    "Max bytes Uploaded":"アップロード可能な最大Mb",
    "Collection did not upload files":"コレクションがファイルをアップロードしていない",

    "Create Offline collector: Select artifacts to collect":"オフライン: 収集するアーティファクトを選択する",
    "Configure Collection":"コレクションの設定",
    "Create Offline Collector: Configure artifact parameters":"オフラインコレクタ作成: アーティファクトのパラメータを設定する",
    "Create Offline Collector: Review request":"オフラインコレクタ作成: リクエストをレビューする",
    "Create Offline Collector: Create collector":"オフラインコレクタ作成: コレクタを作成する",
    "Create Offline collector:  Configure Collector":"オフラインコレクタ作成: コレクタを設定する",
    "Target Operating System":"ターゲットOS",
    "Password":"パスワード",
    "Report Template":"レポートテンプレート",
    "No Report":"レポートなし",
    "Collection Type":"コレクションタイプ",
    "Zip Archive":"Zipアーカイブ",
    "Google Cloud Bucket":"Google Cloudバケット",
    "AWS Bucket":"AWSバケット",
    "SFTP Upload":"SFTPアップロード",
    "Velociraptor Binary":"Velociraptorバイナリ",
    "Temp directory":"一時的なディレクトリ",
    "Temp location":"一時的なロケーション",
    "Compression Level":"圧縮レベル",
    "Output format":"出力形式",
    "CSV and JSON":"CSV y JSON",
    "Output Prefix":"アウトプットプレフィックス",
    "Output filename prefix":"アウトプットファイル名のプレフィックス",

    "DeleteHuntDialog": <>
                    <p>このハントが停止されたら、収集されたデータが永久に削除されます。</p>
                    <p>ハントを停止しますか？</p>
                        </>,

    "Started":"開始された",
    "Expires":"有効期限",
    "Scheduled":"予定されている",
    "New Hunt":"新規ハント",
    "Run Hunt":"ハントの実行",
    "Stop Hunt":"ハントの停止",
    "Delete Hunt":"ハントの削除",
    "Copy Hunt":"ハントのコピー",
    "No hunts exist in the system. You can start a new hunt by clicking the New Hunt button above.":"システム内にハントが存在しません。上の「新規ハント」ボタンをクリックすると、新しいハントを開始することができます。",
    "Please select a hunt above":"上記よりハントを選択してください。",
    "Clients":"クライアント",
    "Notebook for Hunt": hunt_id=>hunt_id + "ハントのノートブック",

    "Hunt ID":"ハントID",
    "Creation Time":"作成時間",
    "Expiry Time":"有効期限",
    "Total scheduled":"調査するクライアント数",
    "Finished clients":"調査済みのクライアント数",
    "Full Download":"完全なダウンロード",
    "Summary Download":"サマリダウンロード",
    "Summary (CSV Only)":"サマリ (CSVのみ)",
    "Summary (JSON Only)":"サマリ (JSONのみ)",
    "name":"名前",
    "size":"サイズ",
    "date":"日付",
    "New Hunt - Configure Hunt":"新規ハント - ハントの設定",
    "Hunt description":"ハントの説明",
    "Expiry":"有効期限",
    "Include Condition":"条件",
    "Run everywhere":"全端末で実行する",
    "Exclude Condition":"除外条件",
    "Configure Hunt":"ハントの設定",
    "Estimated affected clients":"影響を受けるクライアントの推定数",
    "All Known Clients":"すべての既存のクライアント",
    "1 Day actives":"１日以上アクティブ",
    "1 Week actives":"１周間以上アクティブ",
    "1 Month actives":"１ヶ月以上アクティブ",
    "Create Hunt: Select artifacts to collect":"ハント作成: 収集するアーティファクトを選択する",
    "Create Hunt: Configure artifact parameters":"ハント作成: アーティファクトパラメータを設定する",
    "Create Hunt: Specify resource limits":"ハント作成: リソース制限を設定する",
    "Create Hunt: Review request":"ハント作成: リクエストをレビューする",
    "Create Hunt: Launch hunt":"ハント作成: ハントを開始する",

    "ClientId": "クライアントID",
    "StartedTime":"開始された時間",
    "TotalBytes":"総バイト数",
    "TotalRows":"総行数",

    "client_time":"クライアントの時間",
    "level":"レベル",
    "message":"メッセージ",

    "RecursiveVFSMessage": path=><>
       <b>{path}</b>にあるすべてのファイルを再帰的に取得しようとしています。
       <br/><br/>
       これにより、端末から大量のデータが転送される可能性があります。アップロードの上限はデフォルトで1gbですが、収集されたアーティファクトの画面で変更することができます。
    </>,

    "Textview":"Textビュー",
    "HexView":"Hexビュー",
    "Refresh this directory (sync its listing with the client)":"ディレクトリをリフレッシュする (クライアントとリストを同期させる)",
    "Recursively refresh this directory (sync its listing with the client)":"ディレクトリを再帰的にリフレッシュする (クライアントとリストを同期させる)",
    "Recursively download this directory from the client":"クライアントからこのディレクトリを再帰的にダウンロードする",
    "View Collection":"コレクションを開く",
    "Size":"サイズ",
    "Mode":"モード",
    "mtime":"mtime",
    "atime":"atime",
    "ctime":"ctime",
    "btime":"btime",
    "Mtime":"Mtime",
    "Atime":"Atime",
    "Ctime":"Ctime",
    "Btime":"Btime",
    "Properties":"プロパティ",
    "No data available. Refresh directory from client by clicking above.":"データはありません。上記をクリックし、クライアントからディレクトリを更新してください。",
    "Please select a file or a folder to see its details here.":"ファイルやフォルダを選択すると、その詳細が表示されます。",
    "Currently refreshing from the client":"現在、クライアント情報を更新中",
    "Recursively download files":"ファイルを再帰的にダウンロードする",

    "Home":"ホーム",
    "Hunt Manager":"ハントマネージャ",
    "View Artifacts":"アーティファクトの閲覧",
    "Server Events":"サーバイベント",
    "Server Artifacts":"サーバアーティファクト",
    "Host Information":"ホスト情報",
    "Virtual Filesystem":"仮想ファイルシステム",
    "Client Events":"クライアントイベント",
    "This is a notebook for processing a hunt.":"ハントを処理するためのノートブックです。",
    "ToolLocalDesc":
    <>
    ツールは必要に応じてVelociraptorサーバからクライアントに提供されます。
    クライアントはそのツールを自分のディスクにキャッシュし、次に必要になったときにハッシュを比較します。
    ツールは、ハッシュが変更された場合のみダウンロードされます。
    </>,
    "ServedFromURL": (url)=>
    <>
      クライアントは必要に応じて、<a href={api.href(url)}>{url}</a> から直接ツールを取得します。
      もしハッシュが期待されるハッシュと一致しない場合、クライアントはそのファイルを拒否することに注意してください。
    </>,
    "ServedFromGithub": (github_project, github_asset_regex)=>
    <>
      ツールのURLは、<b>{github_asset_regex}</b>にマッチするプロジェクト<b>{github_project}</b>の最新リリースとしてGitHubからリフレッシュされます。
    </>,
    "PlaceHolder":
    <>
      ツールのハッシュは現在不明です。
      初めてツールが必要になったとき、Velociraptorはその上流のURLからツールをダウンロードし、そのハッシュを計算します。
    </>,
    "ToolHash":
    <>
      ツールのハッシュが計算されました。
      クライアントがこのツールを使用する必要がある場合、このハッシュがダウンロードするものと一致することを確認します。
    </>,
    "AdminOverride":
    <>
      ツールは管理者が手動でアップロードしたもので、次回のVelociraptorサーバの更新時には自動的にアップグレードされません。
    </>,
    "ToolError":
    <>
      ツールのハッシュは不明で、URLも定義されていません。
      Velociraptorが解決できないため、このツールをアーティファクトで使用することは不可能になります。
      手動でファイルをアップロードすることはできます。
    </>,
    "OverrideToolDesc":
    <>
      管理者として、そのツールとして使用されるバイナリを手動でアップロードすることができます。
      これは、上流のURL設定をオーバーライドし、それを必要とするすべてのアーティファクトにあなたのツールを提供します。
      別の方法として、クライアントがツールを取得するための URL を設定します。
    </>,

    "Include Labels":"ラベルを含む",
    "Exclude Labels":"ラベルを除く",
    "? for suggestions":"おすすめ: ?",
    "Served from URL":"提供URL",
    "Placeholder Definition":"プレースホルダの定義",
    "Materialize Hash":"ハッシュをマテリアライズする",
    "Tool":"ツール",
    "Override Tool":"ツールのオーバーライド",
    "Select file":"ファイルの選択",
    "Click to upload file":"クリックでファイルをアップロードする",
    "Set Serve URL":"提供URLの設定",
    "Served Locally":"ローカルからの提供",
    "Tool Hash Known":"既存のツールハッシュ",
    "Re-Download File":"ファイルの再ダウンロード",
    'Re-Collect from the client': "クライアントからの再収集",
    'Collect from the client': 'クライアントから収集する',
    "Tool Name":"ツール名",
    "Upstream URL":"上流のURL",
    "Endpoint Filename":"ホストのファイル名",
    "Hash":"ハッシュ",
    "Serve Locally":"ローカルで提供する",
    "Serve URL":"URLを提供する",
    "Fetch from Client": "クライアントから取得する",
    "Last Collected": "最後の収集",
    "Offset": "オフセット",
    "Show All": "すべてを表示する",
    "Recent Hosts": "最近のホスト",
    "Download JSON": "JSONのダウンロード",
    "Download CSV": "CSVのダウンロード",
    "Transform Table": "トランスフォームテーブル",
    "Transformed": "トランスフォーム",

    "Select a notebook from the list above.":"上記のリストからノートブックを選択します。",
    "Cancel":"キャンセル",
    "Recalculate":"再計算",
    "Stop Calculating":"計算の停止",
    "Edit Cell":"セルの編集",
    "Up Cell":"アップセル",
    "Down Cell":"ダウンセル",
    "Add Cell":"セルの追加",
    "Suggestion":"提案",
    "Suggestions":"提案",
    "Add Timeline":"タイムラインの追加",
    "Add Cell From This Cell":"このセルからセルを追加する",
    "Add Cell From Hunt":"ハントからセルを追加する",
    "Add Cell From Flow":"フローからセルを追加する",
    "Rendered":"レンダーされた",
    "Undo":"元に戻す",
    "Delete Cell":"セルの削除",
    "Uptime":"稼働時間",
    "BootTime":"起動時間",
    "Procs":"プロセス",
    "OS":"OS",
    "Platform":"プラットフォーム",
    "PlatformFamily":"プラットフォームファミリ",
    "PlatformVersion":"プラットフォームバージョン",
    "KernelVersion":"カーネルバージョン",
    "VirtualizationSystem":"仮想システム",
    "VirtualizationRole":"仮想ロール",
    "HostID":"ホストID",
    "Exe":"EXE",
    "Fqdn":"FQDN",
    "Create a new Notebook":"新規ノートブック",
    "Collaborators":"協力者",
    "Submit":"サブミットする",
    "Edit notebook ":"ノートブックの編集",
    "Notebook uploads":"ノートブックのアップロード",
    "User Settings":"ユーザ設定",
    "Select a user": "ユーザの選択",

    "Theme":"テーマ",
    "Select a theme":"テーマの選択",
    "Default Velociraptor":"デフォルトのVelociraptor",
    "Velociraptor (light)":"Velociraptor (ライト)",
    "Ncurses (light)":"Ncurses (ライト)",
    "Velociraptor (dark)":"Velociraptor (ダーク)",
    "Github dimmed (dark)":"仄暗いGithub (ダーク)",
    "Cool Gray (dark)":"クールグレー (ダーク)",
    "Strawberry Milkshake (light)":"イチゴミルクシェイク (ライト)",
    "Downloads Password":"ダウンロードのパスワード",
    "Default password to use for downloads":"ダウンロード時に使用するデフォルトパスワード",

    "Create Artifact from VQL":"VQLからアーティファクトを作成する",
    "Member":"メンバー",
    "Response":"レスポンス",
    "Super Timeline":"スーパータイムライン",
    "Super-timeline name":"スーパータイムライン名",
    "Timeline Name":"タイムライン名",
    "Child timeline name":"子タイムライン名",
    "Time column":"時間列",
    "Time Column":"時間列",
    "Language": "言語",
    "Match by label": "ラベルでのマッチング",
    "All known Clients": "既存のクライアント",
    "X per second": x=><>{x}毎秒</>,
    "HumanizeDuration": difference=>{
      if (difference<0) {
          return <>
                   In {humanizeDuration(difference, {
                       round: true,
                       language: "ja",
                   })}
                 </>;
      }
      return <>
               {humanizeDuration(difference, {
                   round: true,
                   language: "ja",
               })} 前
             </>;
    },
    "Transform table": "トランスフォームテーブル",
    "Sort Column": "カラムのソート",
    "Filter Regex": "正規表現でフィルタする",
    "Filter Column": "カラムをフィルタする",
    "Select label to edit its event monitoring table": "ラベルを選択して、そのイベント監視テーブルを編集する",
    "EventMonitoringCard":
    <>
    イベント監視は、特定のラベルグループを対象とします。
    上のラベルグループを選択すると、そのグループを対象とした特定のイベントアーチファクトを設定することができます。
    </>,
    "Event Monitoring: Configure Label groups": "イベント監視: ラベルグループの設定",
    "Configuring Label": "ラベルの設定",
    "Event Monitoring Label Groups": "イベント監視のラベルグループ",
    "Event Monitoring: Select artifacts to collect from label group ": "イベント監視: ラベルグループから収集するアーティファクトを選択する ",
    "Artifact Collected": "収集されたアーティファクト",
    "Event Monitoring: Configure artifact parameters for label group ": "イベント監視: ラベルグループのアーティファクトパラメータを設定する ",
    "Event Monitoring: Review new event tables": "イベント監視: 新規イベントテーブルをレビューする",

    "Server Event Monitoring: Select artifacts to collect on the server":"サーバイベント監視: サーバで収集するアーティファクトを選択する",
    "Server Event Monitoring: Configure artifact parameters for server":"サーバイベント監視: サーバのアーティファクトパラメータを設定する",
    "Server Event Monitoring: Review new event tables":"サーバイベント監視: 新しいイベントテーブルをレビューする",
    "Configure Label Group":"レベルグループを設定する",
    "Select artifact": "アーティファクトの選択",

    "Raw Data":"生データ",
    "Logs":"ログ",
    "Log":"ログ",
    "Report":"レポート",

    "NotebookId":"ノートブックID",
    "Modified Time":"変更時間",
    "Time": "時間",
    "No events": "イベントなし",
    "_ts": "サーバ時間",

    "Timestamp":"タイムスタンプ",
    "started":"開始された",
    "vfs_path":"VFSパス",
    "file_size":"ファイルサイズ",
    "uploaded_size":"アップロードサイズ",

    "Select a language":"言語を選択する",
    "Japanese": "日本語",
    "English":"英語",
    "Deutsch":"ドイツ語",
    "Spanish": "スペイン語",
    "Portuguese": "ポルトガル語",
    "French": "フランス語",

    "Type":"タイプ",
    "Export notebooks":"ノートブックのエクスポート",
    "Export to HTML":"HTMLのエクスポート",
    "Export to Zip":"Zipのエクスポート",

    "Permanently delete Notebook":"ノートブックを永久に削除する",
    "You are about to permanently delete the notebook for this hunt":"このハントのノートブックを永久に削除しようとしています。",

    "Data":"データ",
    "Served from GitHub":"GitHubから提供",
    "Refresh Github":"GitHubをリフレッシュする",
    "Github Project":"GitHubプロジェクト",
    "Github Asset Regex":"GitHubアセット正規表現",
    "Admin Override":"管理者のオーバーライド",
    "Serve from upstream":"上流からの提供",

    "Update server monitoring table":"サーバ監視テーブルの更新",
    "Show server monitoring tables":"サーバ監視テーブルの表示",

    "Display timezone": "タイムゾーンの表示",
    "Select a timezone": "タイムゾーンの選択",

    "Update client monitoring table":"クライアント監視テーブルの更新",
    "Show client monitoring tables":"クライアント監視テーブルの表示",
    "Urgent": "急",
    "Skip queues and run query urgently": "キューをスキップしてクエリを緊急に実行する",

    // Below need verification
    "Role_administrator" : "サーバー管理者",
     "Role_org_admin" : "組織管理者",
     "Role_reader" : "読み取り専用ユーザー",
     "Role_analyst" : "アナリスト",
     "Role_investigator" : "捜査官",
     "Role_artifact_writer" : "アーティファクト ライター",
     "Role_api" : "読み取り専用 API クライアント",

     "Perm_ANY_QUERY": "任意のクエリ",
     "Perm_PUBISH": "公開",
     "Perm_READ_RESULTS": "結果の読み取り",
     "Perm_LABEL_CLIENT": "ラベル クライアント",
     "Perm_COLLECT_CLIENT": "収集クライアント",
     "Perm_START_HUNT": "ハント開始",
     "Perm_COLLECT_SERVER": "収集サーバー",
     "Perm_ARTIFACT_WRITER": "アーティファクトライター",
     "Perm_SERVER_ARTIFACT_WRITER": "サーバー アーティファクト ライター",
     "Perm_EXECVE" : "EXECVE",
     "Perm_NOTEBOOK_EDITOR": "ノートブックエディタ",
     "Perm_SERVER_ADMIN": "サーバー管理者",
     "Perm_ORG_ADMIN": "組織管理者",
     "Perm_IMPERSONATION": "なりすまし",
     "Perm_FILESYSTEM_READ": "ファイルシステムの読み取り",
     "Perm_FILESYSTEM_WRITE": "ファイルシステム書き込み",
     "Perm_MACHINE_STATE": "マシンの状態",
     "Perm_PREPARE_RESULTS": "結果の準備",
     "Perm_DATASTORE_ACCESS": "データストアアクセス",

     "ToolPerm_ANY_QUERY": "すべてのクエリを発行します ",
     "ToolPerm_PUBISH": "サーバー側のキューにイベントを発行します (通常は必要ありません)",
     "ToolPerm_READ_RESULTS": "すでに実行されているハント、フロー、またはノートブックから結果を読み取る",
     "ToolPerm_LABEL_CLIENT": "クライアントのラベルとメタデータを操作できます",
     "ToolPerm_COLLECT_CLIENT": "クライアントでの新しいコレクションのスケジュールまたはキャンセル",
     "ToolPerm_START_HUNT": "新しいハントを開始",
     "ToolPerm_COLLECT_SERVER": "Velociraptor サーバーで新しいアーティファクト コレクションをスケジュールする",
     "ToolPerm_ARTIFACT_WRITER": "サーバー上で実行されるカスタム アーティファクトを追加または編集する",
     "ToolPerm_SERVER_ARTIFACT_WRITER": "サーバー上で実行されるカスタム アーティファクトを追加または編集する",
     "ToolPerm_EXECVE": "クライアントでの任意のコマンドの実行を許可",
     "ToolPerm_NOTEBOOK_EDITOR": "ノートブックとセルの変更を許可",
     "ToolPerm_SERVER_ADMIN": "サーバー構成の管理を許可",
     "ToolPerm_ORG_ADMIN": "組織の管理を許可",
     "ToolPerm_IMPERSONATION": "ユーザーが query() プラグインに別のユーザー名を指定できるようにします",
     "ToolPerm_FILESYSTEM_READ": "ファイルシステムから任意のファイルを読み取ることができます",
     "ToolPerm_FILESYSTEM_WRITE": "ファイルシステムにファイルを作成することを許可",
     "ToolPerm_MACHINE_STATE": "マシンから状態情報を収集することが許可されています (例: pslist())",
     "ToolPerm_PREPARE_RESULTS": "zip ファイルの作成を許可",
     "ToolPerm_DATASTORE_ACCESS": "生データストアへのアクセスを許可",
};

_.each(automated, (v, k)=>{
    Japanese[hex2a(k)] = v;
});

export default Japanese;
