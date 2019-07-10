#!/usr/bin/python

import argparse
import os
import yaml

parser = argparse.ArgumentParser(
    description='Reorganize an artifact directory so paths match with names.')

parser.add_argument('directory', type=str,
                    help="The base path to inspect.")


def main():
    args = parser.parse_args()
    root = args.directory
    file_info = dict()

    for root, dirs, files in os.walk(root, topdown=False):
        for name in files:
            path_name = os.path.join(root, name)
            if path_name.endswith(".yaml") or path_name.endswith(".yml"):
                with open(path_name) as fd:
                    definition = yaml.safe_load(fd)
                    name = definition['name']
                    if name in file_info:
                        raise TypeError("Files %s and %s contain artifact named %s" % (
                            path_name, file_info[name], name))

                    file_info[name] = path_name

    for name, path_name in file_info.items():
        correct_path = os.path.join(root, *name.split(".")) + ".yaml"
        print (correct_path)

        directory = os.path.dirname(correct_path)
        try:
            os.makedirs(directory)
        except: pass

        os.rename(path_name, correct_path)


if __name__ == '__main__':
    main()
