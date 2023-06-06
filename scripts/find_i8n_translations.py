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

def ProcessFile(path):
    translations = dict()

    for root, dirs, files in os.walk(path):
        for name in files:
            if not name.endswith(".jsx"):
                continue
            filename = os.path.join(root, name)
            with open(filename) as fd:
                for line in fd:
                    m = key_regex.search(line)
                    if m:
                        translations[m.group(2)] = True

    discovered = DiscoverTranslations('./gui/velociraptor/src/components/')

    with open(os.path.splitext(path)[0] + ".json") as fd:
        encoded_existing = json.loads(fd.read())
        existing = dict()
        existing_translations = dict()
        for k, v in encoded_existing.items():
            existing[Decode(k)] = True
            existing_translations[Decode(k)] = v

    # The automated translations
    automated = dict()
    for k in discovered:
        if k not in translations and k not in existing:
            automated[Encode(k)] = k

    outfile = os.path.splitext(path)[0] + "_new.json"
    with open(outfile, "w") as outfd:
        outfd.write(json.dumps(automated, sort_keys=True, indent=4))
        print("Wrote json file %s with %d entries" % (outfile, len(automated)))

    outfile = os.path.splitext(path)[0] + "_automated.json"
    with open(outfile, "w") as outfd:
        outfd.write(json.dumps(existing_translations, sort_keys=True, indent=4))
        print("Wrote automated json file %s with %d entries" % (outfile, len(existing_translations)))


if __name__ == "__main__":
    argument_parser = argparse.ArgumentParser()
    argument_parser.add_argument("path", help="Directory path containing .jsx files")

    args = argument_parser.parse_args()
    ProcessFile(args.path)
