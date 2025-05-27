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
import subprocess
import json

BLACKLISTED = ["!ALL.tkape"]

# The following paths are not NTFS files, so they can be read normally.
NOT_NTFS = ["$Recycle.Bin"]

# Some rules are specified in terms of regex instead of globs so we convert them with this lookup table.
REGEX_TO_GLOB = {
    r"*.+\.(db|db-wal|db-shm)": "*.{db,db-wal,db-shm}",
    r"*.+\.(3gp|aa|aac|act|aiff|alac|amr|ape|au|awb|dss|dvf|flac|gsm|iklax|ivs|m4a|m4b|m4p|mmf|mp3|mpc|msv|nmf|ogg|oga|mogg|opus|ra|rm|raw|rf64|sln|tta|voc|vox|wav|wma|wv|webm)": "*.{3gp,aa,aac,act,aiff,alac,amr,ape,au,awb,dss,dvf,flac,gsm,iklax,ivs,m4a,m4b,m4p,mmf,mp3,mpc,msv,nmf,ogg,oga,mogg,opus,ra,rm,raw,rf64,sln,tta,voc,vox,wav,wma,wv,webm}",
    r"*.+\.(xls|xlsx|csv|tsv|xlt|xlm|xlsm|xltx|xltm|xlsb|xla|xlam|xll|xlw|ods|fodp|qpw)": "*.{xls,xlsx,csv,tsv,xlt,xlm,xlsm,xltx,xltm,xlsb,xla,xlam,xll,xlw,ods,fodp,qpw}",
    r"*.+\.(pdf|xps|oxps)": "*.{pdf,xps,oxps}",
    r"*.+\.(ai|bmp|bpg|cdr|cpc|eps|exr|flif|gif|heif|ilbm|ima|jp2|j2k|jpf|jpm|jpg2|j2c|jpc|jpx|mj2jpeg|jpg|jxl|kra|ora|pcx|pgf|pgm|png|pnm|ppm|psb|psd|psp|svg|tga|tiff|webp|xaml|xcf)": "*.{ai,bmp,bpg,cdr,cpc,eps,exr,flif,gif,heif,ilbm,ima,jp2,j2k,jpf,jpm,jpg2,j2c,jpc,jpx,mj2jpeg,jpg,jxl,kra,ora,pcx,pgf,pgm,png,pnm,ppm,psb,psd,psp,svg,tga,tiff,webp,xaml,xcf}",
    r"*.+\.(db*|sqlite*|)": "*.{db,sqlite}*)",
    r"*.+\.(3g2|3gp|amv|asf|avi|drc|flv|f4v|f4p|f4a|f4b|gif|gifv|m4v|mkv|mov|qt|mp4|m4p|mpg|mpeg|m2v|mp2|mpe|mpv|mts|m2ts|ts|mxf|nsv|ogv|ogg|rm|rmvb|roq|svi|viv|vob|webm|wmv|yuv)": "*.{3g2,3gp,amv,asf,avi,drc,flv,f4v,f4p,f4a,f4b,gif,gifv,m4v,mkv,mov,qt,mp4,m4p,mpg,mpeg,m2v,mp2,mpe,mpv,mts,m2ts,ts,mxf,nsv,ogv,ogg,rm,rmvb,roq,svi,viv,vob,webm,wmv,yuv}",
    r"*.+\.(doc|docx|docm|dotx|dotm|docb|dot|wbk|odt|fodt|rtf|wp*|tmd)": "*.{doc,docx,docm,dotx,dotm,docb,dot,wbk,odt,fodt,rtf,wp*,tmd}",
    r".*\.(jpg|mp4|pdf|webp)": "*.{jpg,mp4,pdf,webp}",
    r"*.\b[a-zA-Z0-9_-]{8}\b.compiled": "*.compiled",
}

# Kape Target rules add some undocumented expansions that dont mean
# anything and can not really be expanded in runtime - we just replace
# them with * glob.
fluff_table = {
    "%user%": "*",
    "%users%": "*",
    "%User%": "*",
    "%Users%": "*",
}


def pathsep_converter_win(path):
    return path.replace("/", "\\")

def pathsep_converter_nix(path):
    return path.replace("\\", "/")

def pathsep_converter_identity(path):
    return path

# Kape targets sometimes have a regex instead of a glob - it is not
# trivial to convert a regex to a glob automatically. A regex is not
# generally necessary and just makes life complicated, so we just hard
# code these translations manually and alert when a new regex pops up.
def unregexify(regex):
    res = REGEX_TO_GLOB.get(regex)
    if not res:
        print("Unknown regex file mask: %s" % regex, file=sys.stderr)
        return regex
    return res

# Ensure only backslashes in paths
def normalize_path(path):
    return path.replace("/", "\\", -1)


class KapeContext:
    groups = {}
    rows = [["Id", "Name", "Category", "Glob", "Accessor", "Comment"]]
    kape_files = []

    # Contains each rule keyed by the filename
    kape_data = OrderedDict()
    pathsep_converter = pathsep_converter_identity

    # Mapping between a target+glob to an id
    ids = {}

    # Mapping between used ids and the rule that is using it.
    id_to_rule = {}
    dirty = False
    last_id = 0
    state_file_path = None

    def __init__(self, state_file_path=None):
        self.state_file_path = state_file_path

        # Keep the mapping between rule names and IDs in a state
        # file. This ensures that output remains stable from run to
        # run and reduces churn through commits.
        if self.state_file_path:
            try:
                with open(state_file_path) as fd:
                    self.ids = dict()
                    self.last_id = 0
                    for key, record_id in json.loads(fd.read()).items():
                        if record_id > self.last_id:
                            self.last_id = record_id
                        self.ids[normalize_path(key)] = record_id

            except (IOError, json.decoder.JSONDecodeError) as e:
                pass

    def resolve_id(self, name, glob):
        key = normalize_path(name + glob)
        res = self.ids.get(key)
        if res is None:
            res = self.last_id + 1
            self.dirty = True
            self.last_id = res
            self.ids[key] = res

        return res

    def flush(self):
        if not self.dirty or not self.state_file_path:
            return

        ids = dict()
        for key, record_id in self.ids.items():
            if record_id not in self.id_to_rule:
                continue
            ids[key] = record_id

        with open(self.state_file_path, "w") as outfd:
            outfd.write(json.dumps(ids, sort_keys=True, indent=4))

def read_targets(ctx, project_path):
    for root, dirs, files in sorted(os.walk(
            project_path + "/Targets", topdown=False)):

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

    for name, data in sorted(ctx.kape_data.items()):
        for target in data["Targets"]:
            if not target:
                continue

            # Convert the target description to a glob. This is done
            # using a complicated interactions between various
            # attributes of the target description that are not that
            # well documented. However, they were clarified by the
            # KapeFiles maintainers here
            # https://github.com/EricZimmerman/KapeFiles/issues/1038

            # The Path always represents a directory
            base_glob = target.get("Path", "")
            if not base_glob:
                continue

            # Expand the targets in the glob
            if ".tkape" in base_glob:
                continue

            # If Recursive is specified, it means we recurse into the directory.
            recursive = ""
            if target.get("Recursive") or ctx.kape_data.get("RecreateDirectories"):
                recursive = "/**10"

            # The default FileMask is *
            mask = target.get("FileMask", "*")
            if mask.lower().startswith("regex:"):
                mask = unregexify(mask[6:])

            mask = "/" + mask

            # To simplify the glob reduce suffix of /**10/* to just
            # /**10
            if recursive and mask == "/*":
                mask = ""

            glob = base_glob.rstrip("\\") + recursive + mask

            row_id = ctx.resolve_id(name, glob)
            ctx.groups[name].add(row_id)

            glob = strip_drive(glob)
            glob = remove_fluff(glob)
            glob = ctx.pathsep_converter(glob)

            ctx.id_to_rule[row_id] = target
            ctx.rows.append([
                row_id,
                target["Name"],
                target.get("Category", ""),
                glob,
                find_accessor(glob),
                target.get("Comment", "")])

    for i in range(3):
        for name, data in sorted(ctx.kape_data.items()):
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

                    for dependency in sorted(deps):
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
    for k,v in fluff_table.items():
        glob = glob.replace(k, v)
    return glob

def run(*argv, path):
    try:
        return subprocess.run(
            argv , cwd=path,
            stdout=subprocess.PIPE).stdout.decode('utf-8').strip()
    except Exception:
        return ""


def format(ctx, kape_file_path):
    parameters_str = ""
    rules = [["Group", "RuleIds"]]

    # Get the latest commit date
    kape_latest_date = run(
        'git', 'log', '-1',
        '--date=format:%Y-%m-%dT%T%z', '--format=%ad',
        path=kape_file_path)

    # Get latest commit hash
    kape_latest_hash = run('git', 'rev-parse', '--short', 'HEAD',
                           path=kape_file_path)

    sorted_rows = [ctx.rows[0]] + sorted(ctx.rows[1:], key=lambda x: int(x[0]))

    for k, v in sorted(ctx.groups.items()):
        d = ctx.kape_data[k]

        # Link back to the rules that make up the ids
        ids = ctx.groups.get(k, [])
        desc = []
        for id in ids:
            target = ctx.id_to_rule.get(id)
            name = target["Name"]
            if target and not name in desc:
                desc.append(name)

        parameters_str += "  - name: %s\n    description: \"%s (by %s): %s\"\n    type: bool\n" % (
            sanitize(k),
            d.get("Description"),
            d.get("Author"),
            ", ".join(desc))

        ids = ['%s' % x for x in sorted(v)]
        if len(ids) > 0:
            rules.append([sanitize(k), sorted(v)])

    with open(os.path.dirname(__file__) + "/" + ctx.template, "r") as fp:
        template = fp.read()


    print (template % dict(
        kape_latest_hash=kape_latest_hash,
        kape_latest_date=kape_latest_date,
        parameters=parameters_str,
        rules="\n".join(["      " + x for x in get_csv(rules).splitlines()]),
        csv="\n".join(["      " + x for x in get_csv(sorted_rows).splitlines()]),
    ))


if __name__ == "__main__":
    argument_parser = argparse.ArgumentParser()
    argument_parser.add_argument("--state_file_path", help="Path to a state file")
    argument_parser.add_argument("kape_file_path", help="Path to the KapeFiles project")
    argument_parser.add_argument("-t", "--target", choices=("win",), help="Which template to fill with data")

    args = argument_parser.parse_args()
    ctx = KapeContext(state_file_path=args.state_file_path)

    if args.target == "win":
        ctx.pathsep_converter = pathsep_converter_win
        ctx.template = "templates/kape_files_win.yaml.tpl"

    else:
        # fallback to former default behavior
        ctx.pathsep_converter = pathsep_converter_identity
        ctx.template = "templates/kape_files_win.yaml.tpl"

    read_targets(ctx, args.kape_file_path)
    format(ctx, args.kape_file_path)

    ctx.flush()
