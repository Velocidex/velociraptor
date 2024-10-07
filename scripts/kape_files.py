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


class KapeContext:
    groups = {}
    rows = [["Id", "Name", "Category", "Glob", "Accessor", "Comment"]]
    kape_files = []
    kape_data = OrderedDict()
    pathsep_converter = pathsep_converter_identity

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

            glob = target.get("Path", "")

            if target.get("Recursive") or ctx.kape_data.get("RecreateDirectories"):
                glob = glob.rstrip("\\") + "/**10"

            mask = target.get("FileMask")
            if mask:
                if mask.lower().startswith("regex:"):
                    mask = unregexify(mask[6:])
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

    for k, v in sorted(ctx.groups.items()):
        parameters_str += "  - name: %s\n    description: \"%s (by %s): %s\"\n    type: bool\n" % (
            sanitize(k),
            ctx.kape_data[k].get("Description"),
            ctx.kape_data[k].get("Author"),
            ", ".join(sorted([ctx.rows[x][1] for x in sorted(v)])))

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
        csv="\n".join(["      " + x for x in get_csv(ctx.rows).splitlines()]),
    ))


if __name__ == "__main__":
    argument_parser = argparse.ArgumentParser()
    argument_parser.add_argument("kape_file_path", help="Path to the KapeFiles project")
    argument_parser.add_argument("-t", "--target", choices=("win",), help="Which template to fill with data")

    args = argument_parser.parse_args()

    ctx = KapeContext()

    if args.target == "win":
        ctx.pathsep_converter = pathsep_converter_win
        ctx.template = "templates/kape_files_win.yaml.tpl"

    else:
        # fallback to former default behavior
        ctx.pathsep_converter = pathsep_converter_identity
        ctx.template = "templates/kape_files_win.yaml.tpl"

    read_targets(ctx, args.kape_file_path)
    format(ctx, args.kape_file_path)
