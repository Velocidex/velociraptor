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

def Encode(k):
    return binascii.hexlify(k.encode()).decode()

def Decode(k):
    try:
        return binascii.unhexlify(k.encode()).decode()
    except Exception as e:
        print("While decoding %s" % k)
        raise e

def ProcessFile(filename):
    translations = dict()

    with open(filename) as fd:
        for line in fd:
            m = key_regex.search(line)
            if m:
                translations[m.group(2)] = True

    discovered = DiscoverTranslations('./gui/velociraptor/src/components/')

    print("Loading %s" % filename)
    with open(os.path.splitext(filename)[0] + ".json") as fd:
        encoded_existing = json.loads(fd.read())
        existing = dict()
        existing_translations = dict()

        encoded_existing_new = dict()
        for k, v in encoded_existing.items():
            decoded = Decode(k)
            # Discard translations that are no longer needed.
            if not decoded in discovered:
                print("Translation %s no longer needed" % decoded)
                continue

            existing[decoded] = True
            existing_translations[decoded] = v
            encoded_existing_new[k] = v

    # Update the automated translations if they are no longer needed.
    if len(encoded_existing) != len(encoded_existing_new):
        with open(os.path.splitext(filename)[0] + ".json", "w") as outfd:
            outfd.write(json.dumps(encoded_existing_new, sort_keys=True, indent=4))

    # The automated translations
    automated = dict()
    for k in discovered:
        if not k in translations and not k in existing:
            automated[Encode(k)] = k

    outfile = os.path.splitext(filename)[0] + "_new.json"
    with open(outfile, "w") as outfd:
        outfd.write(json.dumps(automated, sort_keys=True, indent=4))
        print("Wrote json file %s with %d entries" % (outfile, len(automated)))

    outfile = os.path.splitext(filename)[0] + "_automated.json"
    with open(outfile, "w") as outfd:
        outfd.write(json.dumps(existing_translations, sort_keys=True, indent=4))
        print("Wrote automated json file %s with %d entries" % (outfile, len(existing_translations)))

def ProcessDirectory(path):
    for root, dirs, files in os.walk(path):
        for name in files:
            if not name.endswith(".jsx"):
                continue

            jsx_path = os.path.join(root, name)
            json_path = jsx_path.replace(".jsx", ".json")
            if os.access(json_path, os.R_OK|os.W_OK):
                ProcessFile(jsx_path)


if __name__ == "__main__":
    argument_parser = argparse.ArgumentParser()
    argument_parser.add_argument("language_directory", help="Path to the language directory")

    args = argument_parser.parse_args()
    ProcessDirectory(args.language_directory)
