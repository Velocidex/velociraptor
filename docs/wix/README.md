# Customizing Velociraptor deployments.

This directory contains the WiX XML configuration file that can be
used to tailor your Velociraptor deployment. The configuration file
can be used to build a Windows installer package (MSI) which
automatically installs the service.

The advantage of building your own MSI is that you can customize
aspect of the installation like service names, binary names etc. If you
are happy with the default settings it is easier to just use the
official distributed MSI packages.

To build MSI packages you will need to download and install the WIX
distribution from the github page. We currently recommend the 3.14
release series:

https://github.com/wixtoolset/wix3/releases/

Next, follow these steps:

1. Edit the XML file (`velociraptor_amd64.xml` or
   `velociraptor_x86.xml`). Near the top, make sure the version
   variable of the MSI matches the version of Velociraptor you are
   packaging. The version is found in the Product XML tag (e.g. 0.68.0
   corresponds to 0.6.8). NOTE: MSI will refuse to upgrade a version
   which is not higher than an installed version, so you need to
   increment that version number for each newly deployed package - you
   can just increment the last number for each new deployed MSI
   revision.

2. Optional: Generate valid GUIDs to replace all GUIDs in the config
   file. You can use the Linux `uuidgen` program to make new GUIDs.

3. Optional: You can customize the description, comments, service name
   etc. If you decided to rename the binary you can adjust the name in
   the file. NOTE: Make sure your naming is consistent with the actual
   names using inside the config file.

4. Optional: Modify the directory name where the binary will be
   installed.

5. Save the customized XML file in a new directory for this new MSI
   build (e.g. `C:\temp\msi-build`).

6. Place your Velociraptor client configuration file in a subdirectory
   called `output/client.config.yaml`. WiX will package this file into
   the MSI. You can package the placeholder configuration file instead
   which will allow you later to repack the MSI with the real
   configuration file without rebuilding it with WIX.

7. Add the relevant binary into the output subdirectory as
   `output/velociraptor.exe`. This should be the 32 bit version for
   x86 MSI and the 64 bit version for amd64 packages.

8. Place the appropriate build batch file into your custom build
   directory (`build_amd64.bat` or `build_x86.bat`). Execute the batch
   file to generate the custom MSI. This should produce a new
   `velociraptor_XXX.msi` in your build directory.

Test the MSI file by installing and removing it. A simple test is to
run `msiexec /i velociraptor_XXX.msi` to install it.  Uninstall with
`msiexec /x velociraptor_XXX.msi`.  You can now push the MSI using
group policy everywhere in your domain. Test that the MSI can be
properly repacked (see below)

Note: When upgrading, keep the `UpgradeCode` the same to ensure the
old package is uninstalled and the new one is installed.

# Repacking the custom MSI

A new feature is to be able to repack the MSI with an deployment's
client configuration without having to rebuild the MSI (and therefore
without needing to install WIX). This is much easier than having to
have Wix installed and can be done on any operating system.

If you install the official MSI, the placeholder config file will be
installed in place of the `client.config.yaml`. Since the placeholder
is **not** a valid configuration file, Velociraptor will wait before
starting and attempt to reload the file every few seconds. This
provides you the opportunity to manually replace the file at a later
stage with a correctly formatted file specific for your deployment.

However, it is possible to repack a new client configuration file into
the MSI using the artifact `Server.Utils.CreateMSI` or the following
command (i.e. replace the file within the MSI):

```
velociraptor config repack --exe velociraptor.msi client.config.yaml repacked_velociraptor.msi -v
```

In the above, `velociraptor.msi` is the official (or customized)
velociraptor MSI for the correct architecture. The
`client.config.yaml` file is the client configuration as produced by
the configuration wizard.

Repacking replaces the placeholder inside the MSI with the real
configuration file. The new MSI will then automatically install the
correct configuration file.

If you want to build a customized MSI which can also be repacked you
need to use the placeholder config file (located in the
`output/client.config.yaml` file) in the MSI, then the repacking code
can find it and replace it with the final config as needed.

# Official Velociraptor MSI

The standard MSI which is distributed in the Velociraptor releases
contains the placeholder configuration file and so can be repacked
using the above procedure. The standard package will install a service
with the name "Velociraptor Service" into the location "c:\Program
Files\Velociraptor\Velociraptor.exe".
