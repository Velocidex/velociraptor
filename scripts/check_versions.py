#!/usr/bin/python3

"""Compares the current api defined by vql.yaml against past versions.

If a plugin's definition has changed in the current API but the
version number is not incremented, an error is printed and the program
returns with an error status.
"""

import sys
import subprocess
import yaml
import argparse

api_cache = {}
errored_plugins = {}

def get_commits():
    res = []
    output = subprocess.check_output([
        'git', 'log',
        '--pretty=format:%H %ad', '--date=iso',
        './docs/references/vql.yaml'])
    for line in output.decode("utf8").splitlines():
        parts = line.split(" ", 1)
        res.append(dict(hash=parts[0], date=parts[1]))

    return res

def get_api_ref(commit):
    res = api_cache.get(commit["hash"], {})
    if res:
        return res

    print("Loading API at %s" % commit)
    output = subprocess.check_output([
        'git', 'show', commit["hash"]+':./docs/references/vql.yaml'])

    definitions = yaml.safe_load(output)
    for d in definitions:
        res["%s:%s" % (d["type"], d["name"])] = d

    api_cache[commit["hash"]] = res
    return res

def compare_args(name, version, prev, current):
    equal = True
    prev_args = {}
    for d in prev:
        prev_args[d["name"]] = d["type"]

    current_args = {}
    for d in current:
        prev_d = prev_args.get(d["name"])
        if prev_d is None:
            # Only warn once per error.
            if not errored_plugins.get(name):
                print("%s: New arg %s of type %s. Plugin version should be > %s" % (
                    name, d["name"], d["type"], version))
                errored_plugins[name] = 1

            equal = False

    return equal

def compare_apis(prev, current):
    equal = True
    for key, current_def in current.items():
        prev_def = prev.get(key)

        # No previous def, this is a new function.
        if prev_def is None:
            continue

        # We already warned about this plugin, skip it.
        if errored_plugins.get(key):
            continue

        # The new definition has a newer version - it is ok.
        prev_version = prev_def.get("version", 0)
        if current_def.get("version", 0) > prev_version:
            continue

        name = current_def.get("name")
        current_args = current_def.get("args")
        prev_args = prev_def.get("args")

        if current_args and prev_args and compare_args(
                key, prev_version, prev_args, current_args):
            equal = False

    return equal

def read_current_api():
    with open("./docs/references/vql.yaml") as fd:
        definitions = yaml.safe_load(fd.read())

    res = {}
    for d in definitions:
        res["%s:%s" % (d["type"], d["name"])] = d

    return res

if __name__ == "__main__":
    argument_parser = argparse.ArgumentParser()
    argument_parser.add_argument(
        "number_of_commits",
        help="Number of past commits to consider",
        type=int, default="100")

    args = argument_parser.parse_args()

    commits = get_commits()[:args.number_of_commits]
    current_api = read_current_api()
    for commit in commits:
        try:
            prev_commit = get_api_ref(commit)
        except Exception:
            continue

        compare_apis(prev_commit, current_api)

    if errored_plugins:
        print("Found errors: %s" % [sorted(errored_plugins.keys())])
        raise RuntimeError("Errors found")
