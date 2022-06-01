#!/usr/bin/python3

"""Build a template that can be safely copied to Google translate.

Google translate just indiscriminately translates everything in the document but we need to maintain a dict with keys in English and values in the target language.

This very simple program protects the keys by encoding them in hex
then we can feed the document to Google translate and have it
translate all the value in clear text.

Using the --decode flag we can decode the hex back. A bit of manual
checking is usually necessary (around the react functions especially)
but this cuts down the work significantly.

A good idea is to start with one of the fully translated files
(e.g. de.js)

# This will produce a protected js file.

$ build_i8n_template.py de.js > test.js

# Feed to Google translate and get the result into say translated.js

$ build_i8n_template.py --decode translated.js > lang.js

Now check the file for various corruption issues.
"""

import argparse
import re
import sys
import binascii

key_regex = re.compile("(^\s*\")([^\"]+)(\":)", re.S|re.M)
react_key_regex = re.compile("\\{([^\\}]+)\\}", re.S|re.M)

def replace_key(match):
    pre = match.group(1)
    key = match.group(2)
    post = match.group(3)
    return u"%s%s%s" % (pre, binascii.hexlify(key.encode()).decode(), post)

def un_replace_key(match):
    pre = match.group(1)
    key = match.group(2)
    post = match.group(3)
    return u"%s%s%s" % (pre, binascii.unhexlify(key.encode()).decode(), post)


def ParseFile(filename):
    with open(filename) as fd:
        for line in fd:
            line = key_regex.sub(replace_key, line)
            sys.stdout.write(line)

def UnParseFile(filename):
    with open(filename) as fd:
        for line in fd:
            line = key_regex.sub(un_replace_key, line)
            sys.stdout.write(line)

if __name__ == "__main__":
    argument_parser = argparse.ArgumentParser()
    argument_parser.add_argument("filename", help="Template file to open")
    argument_parser.add_argument("--decode", action='store_true')

    args = argument_parser.parse_args()
    if args.decode:
        UnParseFile(args.filename)
    else:
        ParseFile(args.filename)
