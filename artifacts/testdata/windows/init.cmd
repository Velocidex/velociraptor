mklink "c:\users\link" "c:\Windows"
start cmd /c "artifacts\testdata\files\nc.exe -L -p 3889 -s 0.0.0.0"