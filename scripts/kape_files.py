#!/usr/bin/python3
"""Convert KapeFiles to a Velociraptor artifact.

Kape is a popular target file collector used to triage a live
system. It operates by scanning a group of yaml files called Targets
which describe simple rules about how to find these files. Although
Kape is not open source, the rules driving its actions are stored in
the KapeFiles repository on github.

This script converts the Target yaml files within the KapeFiles
repository to a data driven Velociraptor artifact.

Simply point this script at the root a directory containing ".tkape"
files and we will generate the artifact to stdout.

"""

import argparse
import io
import sys
import csv
import re
import os
import yaml

BLACKLISTED = ["!ALL.tkape"]


class KapeContext:
    groups = {}
    rows = [["Id", "Name", "Category", "Glob", "Accessor", "Comment"]]
    kape_files = []
    kape_data = {}

def read_targets(ctx, project_path):
    for root, dirs, files in os.walk(
            project_path + "/Targets", topdown=False):

        for name in files:
            if not name.endswith(".tkape") or name in BLACKLISTED:
                continue

            ctx.kape_files.append(name)

            try:
                full_path = os.path.join(root, name)
                ctx.kape_data[name] = yaml.safe_load(open(full_path).read())
            except Exception as e:
                print ("Unable to parse %s: %s" % (full_path, e))

            ctx.groups[name] = set()

    for name, data in ctx.kape_data.items():
        for target in data["Targets"]:
            glob = target.get("Path", "")

            if target.get("Recursive"):
                glob = glob.rstrip("\\") + "/**10"

            mask = target.get("FileMask")
            if mask:
                glob = glob.rstrip("\\") + "/" + mask

            # Expand the targets in the glob
            if ".tkape" in glob:
                continue

            row_id = len(ctx.rows)
            ctx.groups[name].add(row_id)

            glob = strip_drive(glob)
            glob = remove_fluff(glob)
            ctx.rows.append([
                row_id,
                target["Name"],
                target.get("Category", ""),
                glob,
                find_accessor(glob),
                target.get("Comment", "")])

    for i in range(3):
        for name, data in ctx.kape_data.items():
            for target in data["Targets"]:
                glob = target.get("Path", "")
                if ".tkape" in glob:
                    deps = find_kape_dependency(ctx, glob)
                    if deps is None:
                        sys.stderr.write("Unable to process dependency %s (%s)\n" %
                                         (name, glob))
                        #import pdb; pdb.set_trace()
                        continue

                    for dependency in deps:
                        ctx.groups[name].add(dependency)

def find_accessor(glob):
    if ":" in glob:
        return "ntfs"

    if "$" in glob:
        return "ntfs"

    return "lazy_ntfs"


def find_kape_dependency(ctx, glob):
    """ Case insensitive search for kape dependency."""
    for k, v in ctx.groups.items():
        if k.lower() == glob.lower():
            return v

def sanitize(name):
    name = name.replace(".tkape", "")
    name = re.sub("[^a-zA-Z0-9]", "_", name)
    return name


def strip_drive(name):
    return re.sub("^[a-zA-Z]:(\\\\|/)", "", name)

def get_csv(rows):
    out = io.StringIO()
    writer = csv.writer(out)
    for row in rows:
        writer.writerow(row)

    return out.getvalue()

def remove_fluff(glob):
    return glob.replace('%user%', '*')

def format(ctx):
    template = """name: Windows.KapeFiles.Targets
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
      WHERE get(member=Group)

      -- Filter only the rules in the rule table that have an Id we
      -- want. Targets with $ in their name probably refer to ntfs
      -- special files and so they are designated as ntfs
      -- accessor. Other targets may need ntfs parsing but not
      -- necessary - they are designated with the lazy_ntfs accessor.
      LET rule_specs_ntfs <= SELECT Id, Glob
        FROM parse_csv(filename=KapeRules, accessor="data")
        WHERE Id in array(array=targets.RuleIds) AND Accessor='ntfs'

      LET rule_specs_lazy_ntfs <= SELECT Id, Glob
        FROM parse_csv(filename=KapeRules, accessor="data")
        WHERE Id in array(array=targets.RuleIds) AND Accessor='lazy_ntfs'

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
                   SELECT * FROM Artifact.Windows.Collectors.File(
                      RootDevice=Device,
                      Accessor="ntfs",
                      collectionSpec=rule_specs_ntfs)
               }, b={

                   -- Prefer the auto accessor if possible since it
                   -- will fall back to ntfs if required but otherwise
                   -- will be faster.
                   SELECT * FROM Artifact.Windows.Collectors.File(
                      RootDevice=Device,
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

      {{ $query := print "SELECT SourceFile, Size, Modified, LastAccessed, Created \\
          FROM source(source='All File Metadata')" }}

      <!-- There could be a huge number of rows just to get the count, so we cap at 10000 -->
      {{ $count := Get ( Query (print "LET X = " $query " LIMIT 10000 " \\
         " SELECT 1 AS ALL, count() AS Count FROM X Group BY ALL") | Expand ) \\
         "0.Count" }}

      <!-- If this is a flow show the parameters. -->
      {{ $flow := Query "LET X = SELECT Request.Parameters.env AS \\
         Env FROM flows(client_id=ClientId, flow_id=FlowId)" \\
         "SELECT * FROM foreach(row=X[0].Env, query={ \\
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

"""
    parameters_str = ""
    rules = [["Group", "RuleIds"]]

    for k, v in sorted(ctx.groups.items()):
        parameters_str += "  - name: %s\n    description: \"%s (by %s): %s\"\n    type: bool\n" % (
            sanitize(k),
            ctx.kape_data[k].get("Description"),
            ctx.kape_data[k].get("Author"),
            ", ".join([ctx.rows[x][1] for x in v]))

        ids = ['%s' % x for x in v]
        if len(ids) > 0:
            rules.append([sanitize(k), sorted(v)])

    print (template % dict(
        parameters=parameters_str,
        rules="\n".join(["      " + x for x in get_csv(rules).splitlines()]),
        csv="\n".join(["      " + x for x in get_csv(ctx.rows).splitlines()]),
    ))


if __name__ == "__main__":
    argument_parser = argparse.ArgumentParser()
    argument_parser.add_argument("kape_file_path", help="Path to the KapeFiles project")

    args = argument_parser.parse_args()

    ctx = KapeContext()
    read_targets(ctx, args.kape_file_path)
    format(ctx)
