# Building the resources

The resources are built once using go-winres
https://github.com/tc-hib/go-winres

The syso files are checked into the repo and reused on each build.

```
cd ./docs/
go-winres make
mv rsrc_windows_386.syso rsrc_windows_amd64.syso ../bin/
```
