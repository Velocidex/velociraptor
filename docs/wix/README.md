# Customizing Velociraptor deployments.

This directory contains the WIX XML file that can be used to tailor
your Velociraptor deployment. The configuration file can be used to
build a Windows installer package (MSI) which automatically installs
the service.

First download the WIX distribution from the github page:

http://wixtoolset.org/releases/

Then modify the XML file:

1. First generate valid GUIDs to replace all GUIDs in the config
   file. You can use the linux uuidgen program to make new GUIDs.

2. Next you can customize the description, comments, service name
   etc. If you decided to rename the binary you can adjust the name in
   the file.

3. Modify the directory name where the binary will be installed.

4. Build the msi using wix:

```
F:\Wix>"c:\Program Files (x86)\WiX Toolset v3.11\bin\candle.exe" velociraptor.xml -arch x64
F:\Wix>"c:\Program Files (x86)\WiX Toolset v3.11\bin\light.exe" velociraptor.wixobj
```

Test the MSI file by installing and removing it. You can now push the
MSI using group policy everywhere in your domain.

Note: When upgrading, keep the UpgradeCode the same to ensure the old
package is uninstalled and the new one is installed.