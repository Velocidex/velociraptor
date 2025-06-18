#!/usr/bin/python

import argparse

def MarkConfig(filename):
    with open(filename) as fd:
        data = fd.read()

    # Page size is 0x1000 so we try to get one marker per page
    for offset in range(0x800, len(data), 0x800):
        lhs = data[:offset]
        marker = "## Velociraptor client configuration (%#x)" % offset
        rhs = data[offset+len(marker):]

        data = lhs + marker + rhs

    print(data)

if __name__ == "__main__":
    argument_parser = argparse.ArgumentParser()
    argument_parser.add_argument(
        "config", help="Path to the sample configuration", default=".")

    args = argument_parser.parse_args()
    MarkConfig(args.config)
