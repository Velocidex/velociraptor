#include <windows.h>
#include "dbghelp.h"


int dumpProcess(int pid, char *filename) {
    HANDLE FileHandle;
    HANDLE ProcHandle;

    FileHandle = CreateFile(
        filename,
        GENERIC_READ | GENERIC_WRITE,
        0,
        NULL,
        CREATE_ALWAYS,
        FILE_ATTRIBUTE_HIDDEN,
        NULL
    );

    if (FileHandle == INVALID_HANDLE_VALUE) {
        return (GetLastError());
    }

    ProcHandle = OpenProcess(
        PROCESS_QUERY_INFORMATION|PROCESS_VM_READ|PROCESS_DUP_HANDLE,
        FALSE,
        pid
    );

    if (ProcHandle == NULL) {
            CloseHandle(FileHandle);
            return (GetLastError());
        }

    if (!MiniDumpWriteDump(
            ProcHandle,
            pid,
            FileHandle,
            MiniDumpWithFullMemory,
            NULL,
            NULL,
            NULL
        )) {
        CloseHandle(FileHandle);
        CloseHandle(ProcHandle);
        return (GetLastError());
    }

    CloseHandle(FileHandle);
    CloseHandle(ProcHandle);
    return 0;
}
