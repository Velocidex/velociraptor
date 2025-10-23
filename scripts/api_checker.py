#!/usr/bin/python3

"""Scans the source code for API use that is not allowed.

Some common APIs are wrapped to workaround bugs and issues. This
script quickly checks for direct use of the APIs bypassing the
wrapping.
"""

import argparse
import re
import os
import sys
import binascii
import base64

class Check:
    def __init__(self, re, allowed, replaced):
        self.re = re
        self.allowed = allowed
        self.replaced = replaced


checks = [Check(re=re.compile("ioutil.TempFile"),
                allowed=re.compile("utils/tempfile"),
                replaced="tempfile.TempFile"),
          Check(re=re.compile("ioutil.TempDir"),
                allowed=re.compile("utils/tempfile"),
                replaced="tempfile.TempDir"),
          Check(re=re.compile("os.TempDir"),
                allowed=re.compile("utils/tempfile"),
                replaced="tempfile.TempDir"),

          # gopsutil is wrapped and should not be called directly.
          Check(re=re.compile("github.com/shirou/gopsutil"),
                allowed=re.compile("vql/psutils"),
                replaced="/vql/psutils/"),

          Check(re=re.compile("github.com/alecthomas/assert"),
                allowed=re.compile("vtesting/assert"),
                replaced="/vtesting/assert/"),

          # Wrap goldie for tests.
          Check(re=re.compile("github.com/sebdah/goldie"),
                allowed=re.compile("vtesting/goldie"),
                replaced="/vtesting/goldie/"),

          Check(re=re.compile("httptest.NewTLSServer"),
                allowed=re.compile("vtesting/tls"),
                replaced="/vtesting/NewTLSServer"),

          # We can not use the slices package because it does not
          # exist on go 1.20 so the legacy build will fail.
          Check(re=re.compile("slices\\.(.+)"),
                allowed=re.compile("Use append()"),
                replaced="Use built in operations append and avoid slices package"),

          Check(re=re.compile("math/rand"),
                allowed=re.compile("utils/rand"),
                replaced="/utils/rand"),

          Check(re=re.compile("ioutil.ReadAll"),
                allowed=re.compile("magefile.go|/utils/copy|_test.go$"),
                replaced="utils.ReadAllWithCtx"),
          ]

def DiscoverAPI(path):
    result = []
    for root, dirs, files in os.walk(path):
        for name in files:
            if not name.endswith(".go"):
                continue

            filename = os.path.join(root, name)
            with open(filename) as fd:
                i = 0
                for line in fd:
                    i += 1
                    for c  in checks:
                        m = c.re.search(line)
                        if m:
                            m = c.allowed.search(filename)
                            if m:
                                continue

                            result.append("%s:%s: should use %s" % (
                                filename, i, c.replaced))

    return result

if __name__ == "__main__":
    argument_parser = argparse.ArgumentParser()
    argument_parser.add_argument(
        "source", help="Path to the source directory", default=".")

    args = argument_parser.parse_args()
    for error in DiscoverAPI(args.source):
        print(error)
