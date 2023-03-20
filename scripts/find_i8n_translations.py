#!/usr/bin/python3

"""Finds all untranslated translations in the source tree.

"""

import argparse
import re
import os
import sys
import binascii
import json
import base64

key_regex = re.compile("(^\s*\")([^\"]+)(\":)", re.S|re.M)
translation_regex = re.compile(r"T\(\"([^\"]+)\"\)", re.S|re.M)

def DiscoverTranslations(path):
    result = dict()
    for root, dirs, files in os.walk(path):
        for name in files:
            if not name.endswith(".jsx"):
                continue
            with open(os.path.join(root, name)) as fd:
                for line in fd:
                    m = translation_regex.search(line)
                    if m:
                        result[m.group(1)] = True
    return result

def ProcessFile(filename):
    translations = dict()

    with open(filename) as fd:
        for line in fd:
            m = key_regex.search(line)
            if m:
                translations[m.group(2)] = True

    discovered = DiscoverTranslations('./gui/velociraptor/src/components/')

    # The automated translations
    automated = dict()
    for k in discovered:
        if not k in translations:
            automated[base64.b64encode(k.encode()).decode()] = k

    outfile = os.path.splitext(filename)[0] + ".json"
    with open(outfile, "w") as outfd:
        outfd.write(json.dumps(automated, sort_keys=True, indent=4))
        print("Wrote json file %s" % outfile)

if __name__ == "__main__":
    argument_parser = argparse.ArgumentParser()
    argument_parser.add_argument("language_file", help="Language file to open")

    args = argument_parser.parse_args()
    ProcessFile(args.language_file)
