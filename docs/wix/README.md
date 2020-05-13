# Customizing Velociraptor deployments.

This directory contains the WIX XML file that can be used to tailor
your Velociraptor deployment. The configuration file can be used to
build a Windows installer package (MSI) which automatically installs
the service.

The advantage of building your own MSI is that your config file will
be bundled inside the MSI so you do not need to push it to endpoints -
simply assign the MSI to your endpoints via SCCM or GPO.

To build MSI packages you will need to download the WIX distribution
from the github page (it requires .NET 3.5):

http://wixtoolset.org/releases/

Then modify the XML file if you like:

1. Make sure the version of the MSI matches the version of
   Velociraptor you are packaging. The version is found in the Product
   XML tag (e.g. 0.42.0 corresponds to 0.4.2).

1. Optional: Generate valid GUIDs to replace all GUIDs in the config
   file. You can use the linux uuidgen program to make new GUIDs.

2. Optional: You can customize the description, comments, service name
   etc. If you decided to rename the binary you can adjust the name in
   the file.

3. Optional: Modify the directory name where the binary will be
   installed.

4. Place your deployment configuration file in the build directory
   called output/client.config.yaml. Wix will package this file into
   the MSI.

5. Add the relevant binaries into the output directory
   (`output/velociraptor.exe` or `output/velociraptor_x86.exe`)

6. Build the msi using the provided bat files (`build_custom.bat` or
   `build_x86_custom.bat`):

Test the MSI file by installing and removing it. You can now push the
MSI using group policy everywhere in your domain.

Note: When upgrading, keep the UpgradeCode the same to ensure the old
package is uninstalled and the new one is installed.


# Standard MSI

The standard MSI which is distributed in the Velociraptor releases
does not have any configuration file. This standard package
("velociraptor.xml") will install a service with the name
"Velociraptor Service" into the location "c:\Program
Files\Velociraptor\Velociraptor.exe".

Since the standard MSI does not have any configuration included with
it, the Velociraptor service will start and simply watch its
installation directory for the configuration file. You should copy the
file there by some other means (e.g. using Group Policy Scheduled
Tasks). The configuration file must be placed in
"C:\Program Files\Velociraptor\Velociraptor.config.yaml"
