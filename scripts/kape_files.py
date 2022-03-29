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
from collections import OrderedDict

BLACKLISTED = ["!ALL.tkape",
               "$SDS.tkape", # This one should be fetched via the
                             # Windows.Triage.SDS
               ]

# The following paths are not NTFS files, so they can be read normally.
NOT_NTFS = ["$Recycle.Bin"]


def pathsep_converter_win(path):
    return path.replace("/", "\\")

def pathsep_converter_nix(path):
    return path.replace("\\", "/")

def pathsep_converter_identity(path):
    return path


class KapeContext:
    groups = {}
    rows = [["Id", "Name", "Category", "Glob", "Accessor", "Comment"]]
    kape_files = []
    kape_data = OrderedDict()
    pathsep_converter = pathsep_converter_identity

def read_targets(ctx, project_path):
    for root, dirs, files in os.walk(
            project_path + "/Targets", topdown=False):

        for name in sorted(files):
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

            if target.get("Recursive") or ctx.kape_data.get("RecreateDirectories"):
                glob = glob.rstrip("\\") + "/**10"

            mask = target.get("FileMask")
            if mask:
                glob = glob.rstrip("\\") + "/" + mask

            # If the glob ends with \\ it means that it is a directory
            # and we actually mean to collect all the files in it.
            if glob.endswith("\\"):
                glob += "*"

            # Expand the targets in the glob
            if ".tkape" in glob:
                continue

            row_id = len(ctx.rows)
            ctx.groups[name].add(row_id)

            glob = strip_drive(glob)
            glob = remove_fluff(glob)
            glob = ctx.pathsep_converter(glob)
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

                # Ignore black listed dependency
                if glob in BLACKLISTED:
                    continue

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
    for subtype in NOT_NTFS:
        if subtype in glob:
            return "lazy_ntfs"


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
    parameters_str = ""
    rules = [["Group", "RuleIds"]]

    for k, v in sorted(ctx.groups.items()):
        parameters_str += "  - name: %s\n    description: \"%s (by %s): %s\"\n    type: bool\n" % (
            sanitize(k),
            ctx.kape_data[k].get("Description"),
            ctx.kape_data[k].get("Author"),
            ", ".join(sorted([ctx.rows[x][1] for x in v])))

        ids = ['%s' % x for x in v]
        if len(ids) > 0:
            rules.append([sanitize(k), sorted(v)])

    with open(os.path.dirname(__file__) + "/" + ctx.template, "r") as fp:
        template = fp.read()

    print (template % dict(
        parameters=parameters_str,
        rules="\n".join(["      " + x for x in get_csv(rules).splitlines()]),
        csv="\n".join(["      " + x for x in get_csv(ctx.rows).splitlines()]),
    ))


if __name__ == "__main__":
    argument_parser = argparse.ArgumentParser()
    argument_parser.add_argument("kape_file_path", help="Path to the KapeFiles project")
    argument_parser.add_argument("-t", "--target", choices=("win", "nix"), help="Which template to fill with data")

    args = argument_parser.parse_args()

    ctx = KapeContext()

    if args.target == "win":
        ctx.pathsep_converter = pathsep_converter_win
        ctx.template = "templates/kape_files_win.yaml.tpl"
    elif args.target == "nix":
        ctx.pathsep_converter = pathsep_converter_nix
        ctx.template = "templates/kape_files_nix.yaml.tpl"
    else:
        # fallback to former default behavior
        ctx.pathsep_converter = pathsep_converter_identity
        ctx.template = "templates/kape_files_win.yaml.tpl"

    read_targets(ctx, args.kape_file_path)
    format(ctx)
