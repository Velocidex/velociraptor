#!/usr/bin/python3

"""Convert SQLECmd maps to a VQL artifact."""

import argparse
import os
import re
import yaml
import json

ExcludedCSVPrefixRegex = re.compile("iOS|pCloud|Android")
ExcludedTKapeTargetsRegex = re.compile("XP")
ExcludedTKapeGlobsRegex = re.compile("regex:|(txt|xml|log)$")

Preamble = """name: Generic.Collectors.SQLECmd
description: |
  Many applications maintain internal state using SQLite
  databases. The SQLECmd project is an open source resource for known
  applications and the type of forensic information we can recover.

  ## NOTES

  1. This artifact is automatically generated from the SQLECmd project
  2. This artifact uses the SQLite library, since the library does not
     support accurate CPU limits, this artifact can use a lot of CPU
     despite a CPU limit specified.
  3. Locked or in use SQLite files will be copied to a tempfile and
     then queried.

  4. If UseFilenames is enabled we only look at known
     filenames. Disabling it will try to identify all sqlite files
     within the search glob. This is slower but may find more
     potential files (e.g. renamed).

reference:
  - https://github.com/EricZimmerman/SQLECmd

export: |
  LET Identify(Query, OSPath, IdentifyValue) = SELECT {
      SELECT *
      FROM sqlite(file=OSPath, query=Query)
    } AS Hits
  FROM scope()
  WHERE Hits = IdentifyValue

  LET ApplyFile(IdentifyQuery, SQLQuery, IdentifyValue) = SELECT *
    FROM foreach(row=SQLiteFiles,
    query={
      SELECT * FROM if(
        condition=Identify(Query=IdentifyQuery, OSPath=OSPath, IdentifyValue=IdentifyValue),
        then={
            SELECT *, OSPath FROM sqlite(file=OSPath, query=SQLQuery)
        })
  })

parameters:
- name: GlobExpr
  description: A glob to search for SQLite files.
  type: csv
  default: |
    Name,Glob
%(Globs)s
- name: Accessor
  default: auto
- name: UseFilenames
  default: Y
  type: bool
  description: When set use filenames to optimize identification of files.
- name: AlsoUpload
  description: Also upload the raw sqlite files
  type: bool

sources:
- query: |
   LET AllFilenamesRegex <= '''%(AllFilenamesRegex)s'''
   LET SQLiteFiles <=
   SELECT OSPath,
    read_file(filename=OSPath, length=15, accessor=Accessor) AS Magic,
    if(condition=AlsoUpload,
       then=upload(file=OSPath,
                   mtime=Mtime,
                   atime=Atime,
                   ctime=Ctime,
                   btime=Btime)) AS Upload
   FROM glob(globs=GlobExpr, accessor=Accessor)
   WHERE NOT IsDir
     AND if(condition=UseFilenames, then=Name =~ AllFilenamesRegex, else=TRUE)
     AND Magic =~ "SQLite format 3"

   SELECT * FROM SQLiteFiles
"""

SourceTemplate = """
- name: %(Name)s
  query: |
    LET IdentifyQuery = '''%(IdentifyQuery)s'''
    LET IdentifyValue = %(IdentifyValue)d
    LET SQLQuery = '''%(Query)s'''
    LET FileName = '''%(FileName)s'''

    SELECT * FROM ApplyFile(
      SQLQuery=SQLQuery, IdentifyQuery=IdentifyQuery, IdentifyValue=IdentifyValue)
"""

def indent(text):
    return re.sub("^", "    ", text, flags=re.S|re.M).strip()

class SQLECmdContext(object):
    def __init__(self, project_path, kapefiles_path, output):
        self.project_path = project_path
        self.kapefiles_path = kapefiles_path
        self.maps = []
        self.map_data = dict()
        self.fd = open(output, "w+")
        self.globs = []
        self.tkape_names = dict()

    tkape_re = re.compile(
        "https://github.com/EricZimmerman/KapeFiles/blob/master/(.+tkape)", re.I)

    def maybe_find_tkape(self, data):
        m = self.tkape_re.search(data)
        if m:
            print("Found tkape: %s" % m.group(0))
            try:
                basename = os.path.basename(m.group(1))
                basename = os.path.splitext(basename)[0]
                with open(os.path.join(self.kapefiles_path, m.group(1))) as fd:
                    self.process_tkape(basename, fd.read())
            except IOError:
                return

    def process_tkape(self, filename, data):
        tkape = yaml.safe_load(data)
        for target in tkape["Targets"]:
            name = filename + ":" + target["Name"]
            if ExcludedTKapeTargetsRegex.search(name):
                continue

            if name in self.tkape_names:
                continue

            self.tkape_names[name] = True

            glob = target.get("Path", "")

            if target.get("Recursive") or tkape.get("RecreateDirectories"):
                glob = glob.rstrip("\\") + "/**10"

            mask = target.get("FileMask")
            if mask:
                glob = glob.rstrip("\\") + "/" + mask

            # If the glob ends with \\ it means that it is a directory
            # and we actually mean to collect all the files in it.
            if glob.endswith("\\"):
                glob += "*"

            if ExcludedTKapeGlobsRegex.search(glob):
                continue

            glob = re.sub("%user%", "*", glob, re.I)

            self.globs.append(dict(name=name, glob=glob))

    def format_globs(self):
        result = ['    "%s","%s"' % (x["name"], x["glob"]) for x in self.globs]
        return "\n".join(result)

    def read_maps(self):
        for root, dirs, files in os.walk(os.path.join(
                self.project_path, "SQLMap/Maps")):
            for name in sorted(files):
                if not name.endswith(".smap"):
                    continue

                self.maps.append(name)

                try:
                    full_path = os.path.join(root, name)
                    data = open(full_path).read()
                    self.maybe_find_tkape(data)

                    desc = yaml.safe_load(data)
                    if ExcludedCSVPrefixRegex.match(desc.get("CSVPrefix")):
                        continue

                    ctx.map_data[name] = desc
                except Exception as e:
                    print ("Unable to parse %s: %s" % (full_path, e))

    def output_vql(self):
        all_filenames = [map.get("FileName")
                         for map in self.map_data.values()
                         if map.get("FileName")]
        self.fd.write(Preamble % dict(
            Globs=self.format_globs(),
            AllFilenamesRegex="^(" + "|".join(all_filenames) + ")$"))

        items = []
        for map_name, map in self.map_data.items():
            for query in map.get("Queries", []):
                items.append(dict(IdentifyQuery=indent(map.get("IdentifyQuery")),
                                  IdentifyValue=map.get("IdentifyValue"),
                                  Description=map.get("Description"),
                                  FileName=map.get("FileName", "."),
                                  Name=query["Name"],
                                  Query=indent(query["Query"])))

        # Sort by name for stable output
        items.sort(key=lambda x: x["Description"])

        for item in items:
            self.fd.write(SourceTemplate % item)

if __name__ == "__main__":
    argument_parser = argparse.ArgumentParser()
    argument_parser.add_argument("sqlecmd_file_path", help="Path to the SQLECmd project")
    argument_parser.add_argument("kapefiles_file_path", help="Path to the KapeFiles project")
    argument_parser.add_argument("output", help="Path to the output yamls file")

    args = argument_parser.parse_args()

    ctx = SQLECmdContext(
        project_path=args.sqlecmd_file_path,
        kapefiles_path=args.kapefiles_file_path,
        output=args.output)
    ctx.read_maps()
    ctx.output_vql()
