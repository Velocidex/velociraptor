#!/usr/bin/python

import argparse
import os
import subprocess
import shutil
import tempfile

parser = argparse.ArgumentParser(
    description='Repack the velocigrr package with a new config file.')

parser.add_argument('config_file', type=str,
                    help="The config file to embed.")

parser.add_argument('deb_package', type=str,
                    help="The path to the velociraptor deb.")


class TempDirectory(object):
    """A self cleaning temporary directory."""

    def __enter__(self):
        self.name = tempfile.mkdtemp()

        return self.name

    def __exit__(self, exc_type, exc_value, traceback):
        shutil.rmtree(self.name, True)


def main():
    args = parser.parse_args()
    if not args.deb_package.endswith("deb"):
        raise RuntimeError("Expeting a debian package not %s:" % args.deb_package)

    with open(args.config_file) as fd:
        config_lines = list(fd.readlines())

    with TempDirectory() as temp_dir_name:
        deb_package = os.path.abspath(args.deb_package)

        subprocess.check_call(
            "ar p " + deb_package + " control.tar.gz | tar -xz",
            shell=True, cwd=temp_dir_name)

        with open(os.path.join(temp_dir_name, "postinst")) as fd:
            postinst_lines = list(fd.readlines())

        # Inject the config into the postinst script.
        new_postinst = (
            [postinst_lines[0],
             "cat << EOF > /etc/velociraptor.config.yaml\n"] +
            config_lines + ["EOF\n\n"] + postinst_lines[1:])

        with open(os.path.join(temp_dir_name, "postinst"), "wt") as fd:
            fd.write("".join(new_postinst))

        subprocess.check_call("tar czf control.tar.gz *[!z]",
                              shell=True, cwd=temp_dir_name)

        subprocess.check_call(["cp", deb_package, deb_package + "_repacked.deb"],
                              cwd=temp_dir_name)

        subprocess.check_call(["ar", "r", deb_package + "_repacked.deb", "control.tar.gz"],
                              cwd=temp_dir_name)


if __name__ == '__main__':
    main()
