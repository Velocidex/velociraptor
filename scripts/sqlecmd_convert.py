#!/usr/bin/python3

"""Convert SQLECmd maps to a VQL artifact."""

import argparse
import os
import re
import yaml
import json

Preamble = '''name: Generic.Collectors.SQLECmd
description: |
  Many applications maintain internal state using SQLite
  databases. The SQLECmd project is an open source resource for known
  applications and the type of forensic information we can recover.

  ## NOTES

  1. This artifact is automatically generated from the SQLECmd project
  2. This artifact uses the SQLite library, since the library does not
     support accurate CPU limits, this artifact can use a lot of CPU
     despite a CPU limit specified.

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
  default: "C:/Users/*/AppData/Local/Google/Chrome/User Data/**"
- name: Accessor
  default: auto
- name: AlsoUpload
  description: Also upload the raw sqlite files
  type: bool

sources:
- query: |
   LET SQLiteFiles <=
   SELECT OSPath,
    read_file(filename=OSPath, length=15, accessor=Accessor) AS Magic,
    if(condition=AlsoUpload, then=upload(file=OSPath)) AS Upload
   FROM glob(globs=GlobExpr, accessor=Accessor)
   WHERE NOT IsDir AND Magic =~ "SQLite format 3"

   SELECT * FROM SQLiteFiles
'''

SourceTemplate = """
- name: %(Name)s
  query: |
    LET IdentifyQuery = '''%(IdentifyQuery)s'''
    LET IdentifyValue = %(IdentifyValue)d
    LET SQLQuery = '''%(Query)s'''

    SELECT * FROM ApplyFile(
      SQLQuery=SQLQuery, IdentifyQuery=IdentifyQuery, IdentifyValue=IdentifyValue)
"""

def indent(text):
    return re.sub("^", "    ", text, flags=re.S|re.M).strip()

class SQLECmdContext(object):
    def __init__(self, project_path):
        self.project_path = project_path
        self.maps = []
        self.map_data = dict()

    def read_maps(self):
        for root, dirs, files in os.walk(os.path.join(
                self.project_path, "SQLMap/Maps")):
            for name in sorted(files):
                if not name.endswith(".smap"):
                    continue

                self.maps.append(name)

                try:
                    full_path = os.path.join(root, name)
                    ctx.map_data[name] = yaml.safe_load(open(full_path).read())
                except Exception as e:
                    print ("Unable to parse %s: %s" % (full_path, e))

    def output_vql(self):
        print(Preamble)
        items = []
        for map_name, map in self.map_data.items():
            for query in map.get("Queries", []):
                items.append(dict(IdentifyQuery=indent(map.get("IdentifyQuery")),
                                  IdentifyValue=map.get("IdentifyValue"),
                                  Description=map.get("Description"),
                                  Name=query["Name"],
                                  Query=indent(query["Query"])))

        # Sort by name for stable output
        items.sort(key=lambda x: x["Description"])

        for item in items:
            print (SourceTemplate % item)

if __name__ == "__main__":
    argument_parser = argparse.ArgumentParser()
    argument_parser.add_argument("sqlecmd_file_path", help="Path to the SQLECmd project")

    args = argument_parser.parse_args()

    ctx = SQLECmdContext(project_path=args.sqlecmd_file_path)
    ctx.read_maps()
    ctx.output_vql()
