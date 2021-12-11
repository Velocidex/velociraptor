name: Windows.KapeFiles.Targets
description: |

    Kape is a popular bulk collector tool for triaging a system
    quickly. While KAPE itself is not an opensource tool, the logic it
    uses to decide which files to collect is encoded in YAML files
    hosted on the KapeFiles project
    (https://github.com/EricZimmerman/KapeFiles) and released under an
    MIT license.

    This artifact is automatically generated from these YAML files,
    contributed and maintained by the community. This artifact only
    encapsulates the KAPE "Targets" - basically a bunch of glob
    expressions used for collecting files on the endpoint. We do not
    do any post processing these files - we just collect them.

    We recommend that timeouts and upload limits be used
    conservatively with this artifact because we can upload really
    vast quantities of data very quickly.

reference:
  - https://www.kroll.com/en/insights/publications/cyber/kroll-artifact-parser-extractor-kape
  - https://github.com/EricZimmerman/KapeFiles

parameters:
  - name: UseAutoAccessor
    description: Uses file accessor when possible instead of ntfs parser - this is much faster.
    type: bool
    default: Y
  - name: Device
    description: Name of the drive letter to search.
    default: "C:"
  - name: VSSAnalysis
    type: bool
    default:
    description: If set we run the collection across all VSS and collect only unique changes.

%(parameters)s
  - name: KapeRules
    type: hidden
    description: A CSV file controlling the different Kape Target Rules
    default: |
%(csv)s
  - name: KapeTargets
    type: hidden
    description: Each parameter above represents a group of rules to be triggered. This table specifies which rule IDs will be included when the parameter is checked.
    default: |
%(rules)s
  - name: DontBeLazy
    description: Normally we prefer to use lazy_ntfs for speed. Sometimes this might miss stuff so setting this will fallback to the regular ntfs accessor.
    type: bool

sources:
  - name: All File Metadata
    query: |
      -- Select all the rule Ids to be included depending on the group
      -- selection.
      LET targets <= SELECT * FROM parse_csv(
           filename=KapeTargets, accessor="data")
      WHERE get(member=Group) AND log(message="Selecting " + Group)

      -- Filter only the rules in the rule table that have an Id we
      -- want. Targets with $ in their name probably refer to ntfs
      -- special files and so they are designated as ntfs
      -- accessor. Other targets may need ntfs parsing but not
      -- necessary - they are designated with the lazy_ntfs accessor.
      LET rule_specs_ntfs <= SELECT Id, Glob
        FROM parse_csv(filename=KapeRules, accessor="data")
        WHERE Id in array(array=targets.RuleIds) AND Accessor='ntfs'
        AND log(message="ntfs: Selecting glob " + Glob)

      LET rule_specs_lazy_ntfs <= SELECT Id, Glob
        FROM parse_csv(filename=KapeRules, accessor="data")
        WHERE Id in array(array=targets.RuleIds) AND Accessor='lazy_ntfs'
        AND log(message="auto: Selecting glob " + Glob)

      -- Call the generic VSS file collector with the globs we want in
      -- a new CSV file.
      LET all_results <= SELECT * FROM if(
           condition=VSSAnalysis,
           then={
             SELECT * FROM chain(
               a={
                   -- For VSS we always need to parse NTFS
                   SELECT * FROM Artifact.Windows.Collectors.VSS(
                      RootDevice=Device, Accessor="ntfs",
                      collectionSpec=rule_specs_ntfs)
               }, b={
                   SELECT * FROM Artifact.Windows.Collectors.VSS(
                      RootDevice=Device,
                      Accessor=if(condition=DontBeLazy,
                                  then="ntfs", else="lazy_ntfs"),
                      collectionSpec=rule_specs_lazy_ntfs)
               })
           }, else={
             SELECT * FROM chain(
               a={

                   -- Special files we access with the ntfs parser.
                   SELECT * FROM Artifact.Generic.Collectors.File(
                      Root=Device,
                      Accessor="ntfs",
                      collectionSpec=rule_specs_ntfs)
               }, b={

                   -- Prefer the auto accessor if possible since it
                   -- will fall back to ntfs if required but otherwise
                   -- will be faster.
                   SELECT * FROM Artifact.Generic.Collectors.File(
                      Root=Device,
                      Accessor=if(condition=UseAutoAccessor,
                                  then="auto", else="lazy_ntfs"),
                      collectionSpec=rule_specs_lazy_ntfs)
               })
           })

      SELECT * FROM all_results WHERE _Source =~ "Metadata"

  - name: Uploads
    query: |
      SELECT * FROM all_results WHERE _Source =~ "Uploads"

reports:
  - type: CLIENT
    template: |
      {{ import "Reporting.Default" "Templates" }}

      <!-- Default report in case the artifact does not have one -->
      ## {{ .Name }}

      {{ $name := .Name }}

      {{ template "hidden_paragraph_start" dict "description" "View Artifact Description" }}

      {{ Markdown .Description }}

      ### References</h3>

      {{ range .Reference }}
      * [{{ . }}]({{.}})
      {{- end }}

      {{ template "hidden_paragraph_end" }}

      {{ $query := print "SELECT SourceFile, Size, Modified, LastAccessed, Created \
          FROM source(source='All File Metadata')" }}

      <!-- There could be a huge number of rows just to get the count, so we cap at 10000 -->
      {{ $count := Get ( Query (print "LET X = " $query " LIMIT 10000 " \
         " SELECT 1 AS ALL, count() AS Count FROM X Group BY ALL") | Expand ) \
         "0.Count" 0 }}

      <!-- If this is a flow show the parameters. -->
      {{ $flow := Query "LET X = SELECT Request.Parameters.env AS \
         Env FROM flows(client_id=ClientId, flow_id=FlowId)" \
         "SELECT * FROM foreach(row=X[0].Env, query={ \
             SELECT Key, Value FROM scope()})" | Expand }}

      {{ if $flow }}

      ### Selected Targets

      {{- range $flow -}}{{- if eq (Get . "Value") "Y" }}
      * {{ Get . "Key" }}
      {{- end -}}{{- end }}
      {{ end }}

      ## Files collected

      {{ if gt $count 9999 }}
      Collected more than {{ $count }} files.
      {{ else }}
      Collected a total of {{ $count }} files.
      {{ end }}

      {{ Query $query | Table }}
