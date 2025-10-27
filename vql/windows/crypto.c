// +build windows

// References: Example C Program: Listing the Certificates in a Store
// https://docs.microsoft.com/en-us/windows/desktop/seccrypto/example-c-program-listing-the-certificates-in-a-store
// https://github.com/facebook/osquery/blob/master/osquery/tables/system/windows/certificates.cpp

#include <windows.h>
#include <stdint.h>
#include <wchar.h>
#include <stdio.h>
#include <wincrypt.h>


/// A struct holding the arguments we pass to the WinAPI callback function
typedef struct _ENUM_ARG {
    DWORD dwFlags;
    void* pvStoreLocationPara;
    void *context;
    LPCWSTR storeLocation;
} ENUM_ARG, *PENUM_ARG;

// The GO callback which will receive all the data.
int cert_walker(char *cert, int length,
                LPCWSTR store, int store_len, int *arg);

BOOL WINAPI certEnumSystemStoreCallback(const void* systemStore,
                                        unsigned long flags,
                                        PCERT_SYSTEM_STORE_INFO storeInfo,
                                        void* reserved,
                                        void* arg) {
    ENUM_ARG *enumArg = (ENUM_ARG *)arg;
    PCCERT_CONTEXT   pCertContext=NULL;
    LPCWSTR store_name = (LPCWSTR)systemStore;
    HCERTSTORE hCertStore = CertOpenSystemStoreW(0, store_name);
    if (hCertStore == NULL) {
        printf("Failed to open cert store %S with %ld\n",
               (wchar_t *)systemStore, (uint32_t)GetLastError());
        return FALSE;
    }

    while(1) {
        pCertContext=CertEnumCertificatesInStore(hCertStore, pCertContext);
        if (pCertContext == NULL) {
            break;
        }

        // Just pass the certificate to Go - we will deal with it
        // there.
        cert_walker((char *)pCertContext->pbCertEncoded,
                    (int)pCertContext->cbCertEncoded,
                    store_name, wcslen(store_name),
                    (int *)enumArg->context);
    }
    CertFreeCertificateContext(pCertContext);
    return CertCloseStore(hCertStore, 0);
}


BOOL WINAPI certEnumSystemStoreLocationsCallback(LPCWSTR storeLocation,
                                                 unsigned long flags,
                                                 void* reserved,
                                                 void* arg) {
    int res;
    ENUM_ARG *enumArg = (ENUM_ARG *)arg;
    enumArg->storeLocation = storeLocation;

    flags &= CERT_SYSTEM_STORE_MASK;
    flags |= enumArg->dwFlags & ~CERT_SYSTEM_STORE_LOCATION_MASK;
    return CertEnumSystemStore(flags,
                               enumArg->pvStoreLocationPara,
                               enumArg,
                               certEnumSystemStoreCallback);
}

int get_all_certs(void *context) {
    unsigned long flags = 0;
    int ret = 0;
    DWORD locationId = CERT_SYSTEM_STORE_CURRENT_USER_ID;
    ENUM_ARG enumArg;

    enumArg.dwFlags = flags;
    enumArg.pvStoreLocationPara = NULL;
    enumArg.context = context;

    flags &= ~CERT_SYSTEM_STORE_LOCATION_MASK;
    flags |= (locationId << CERT_SYSTEM_STORE_LOCATION_SHIFT) &
        CERT_SYSTEM_STORE_LOCATION_MASK;

    return CertEnumSystemStoreLocation(
        flags, &enumArg, certEnumSystemStoreLocationsCallback);
}
