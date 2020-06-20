mklink "c:\users\link" "c:\Windows"

# VSS testing.
echo "test" > c:\Users\test.txt
echo "test2" > c:\Users\test2.txt
sc.exe create TestingDetection1 binPath="%COMSPEC% /Q /c echo 'COMSPEC testing 1"
wmic shadowcopy call create Volume='C:'
wevtutil.exe cl System
echo "test2" >> c:\Users\test2.txt
sc.exe create TestingDetection2 binPath="%COMSPEC% /Q /c echo 'COMSPEC testing 2"
wmic shadowcopy call create Volume='C:'