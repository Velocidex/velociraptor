// +build windows

// based on https://support.microsoft.com/en-us/help/323809/how-to-get-information-from-authenticode-signed-executables

/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

#include <windows.h>
#include <wchar.h>
#include <stdio.h>
#include <wincrypt.h>
#include <wintrust.h>
#include <softpub.h>
#include "authenticode.h"

#define ENCODING (X509_ASN_ENCODING | PKCS_7_ASN_ENCODING)

int GetProgAndPublisherInfo(PCMSG_SIGNER_INFO pSignerInfo,
                            authenticode_data_struct* data);
int GetDateOfTimeStamp(PCMSG_SIGNER_INFO pSignerInfo, SYSTEMTIME *st);
int PrintCertificateInfo(PCCERT_CONTEXT pCertContext);
int GetTimeStampSignerInfo(PCMSG_SIGNER_INFO pSignerInfo,
                            PCMSG_SIGNER_INFO *pCounterSignerInfo);
LPWSTR AllocateAndCopyWideString(LPCWSTR inputString);
char *VerifyEmbeddedSignature(LPCWSTR pwszSourceFile);

char *AllocateAndFormatSerialNumber(PCCERT_CONTEXT pCertContext) {
    DWORD dwData = pCertContext->pCertInfo->SerialNumber.cbData;
    char *result = (char *)malloc(dwData * 4);

    if (result != NULL) {
        for (DWORD n = 0; n < dwData; n++) {
            snprintf(result+n*2, 4, "%02x",
                     pCertContext->pCertInfo->SerialNumber.pbData[dwData - (n + 1)]);
        }
    }
    return result;
}

char *AllocateAndGetIssuerName(PCCERT_CONTEXT pCertContext) {
    char *result;
    DWORD dwData;

    // Get Issuer name size.
    if (!(dwData = CertGetNameString(pCertContext,
                                     CERT_NAME_SIMPLE_DISPLAY_TYPE,
                                     CERT_NAME_ISSUER_FLAG,
                                     NULL,
                                     NULL,
                                     0))) {
        return NULL;
    }

    // Allocate memory for Issuer name.
    result = (LPTSTR)malloc(dwData * sizeof(TCHAR));
    if (!result) {
        return NULL;
    }

    // Get Issuer name.
    if (!(CertGetNameString(pCertContext,
                            CERT_NAME_SIMPLE_DISPLAY_TYPE,
                            CERT_NAME_ISSUER_FLAG,
                            NULL,
                            result,
                            dwData))) {
        goto error;
    }

    return result;

 error:
    free(result);
    return NULL;
}

char *AllocateAndGetSubjectName(PCCERT_CONTEXT pCertContext) {
    char *result;
    DWORD dwData;

    // Get Subject name size.
    if (!(dwData = CertGetNameString(pCertContext,
                                     CERT_NAME_SIMPLE_DISPLAY_TYPE,
                                     0,
                                     NULL,
                                     NULL,
                                     0))) {
        return NULL;
    }

    // Allocate memory for subject name.
    result = (LPTSTR)malloc(dwData * sizeof(TCHAR));
    if (!result) {
        return NULL;
    }

    // Get subject name.
    if (!(CertGetNameString(pCertContext,
                            CERT_NAME_SIMPLE_DISPLAY_TYPE,
                            0,
                            NULL,
                            result,
                            dwData))) {
        goto error;
    }

    return result;

 error:
    free(result);
    return NULL;
}


void free_authenticode_data_struct(authenticode_data_struct *data) {
    free(data->filename);
    free(data->program_name);
    free(data->publisher_link);
    free(data->more_info_link);
    free(data->signer_cert_serial_number);
    free(data->issuer_name);
    free(data->subject_name);
    free(data->timestamp_issuer_name);
    free(data->timestamp_subject_name);
    free(data->timestamp);
}


int verify_file_authenticode(wchar_t *filename, authenticode_data_struct* result) {
    int ret = 0;
    HCERTSTORE hStore = NULL;
    HCRYPTMSG hMsg = NULL;
    PCCERT_CONTEXT pCertContext = NULL;
    BOOL fResult;
    DWORD dwEncoding, dwContentType, dwFormatType;
    PCMSG_SIGNER_INFO pSignerInfo = NULL;
    PCMSG_SIGNER_INFO pCounterSignerInfo = NULL;
    DWORD dwSignerInfo;
    CERT_INFO CertInfo;
    SYSTEMTIME st;

    result->filename = AllocateAndCopyWideString(filename);
    result->trusted = "unsigned";

    // Get message handle and store handle from the signed file.
    fResult = CryptQueryObject(CERT_QUERY_OBJECT_FILE,
                               filename,
                               CERT_QUERY_CONTENT_FLAG_PKCS7_SIGNED_EMBED,
                               CERT_QUERY_FORMAT_FLAG_BINARY,
                               0,
                               &dwEncoding,
                               &dwContentType,
                               &dwFormatType,
                               &hStore,
                               &hMsg,
                               NULL);
    if (!fResult) {
        ret = GetLastError();
        goto error;
    }

    // Get signer information size.
    fResult = CryptMsgGetParam(hMsg,
                               CMSG_SIGNER_INFO_PARAM,
                               0,
                               NULL,
                               &dwSignerInfo);
    if (!fResult) {
        goto error;
    }

    // Allocate memory for signer information.
    pSignerInfo = (PCMSG_SIGNER_INFO)malloc(dwSignerInfo);
    if (!pSignerInfo) {
        ret = -1;
        goto error;
    }

    // Get Signer Information.
    fResult = CryptMsgGetParam(hMsg,
                               CMSG_SIGNER_INFO_PARAM,
                               0,
                               (PVOID)pSignerInfo,
                               &dwSignerInfo);
    if (!fResult) {
        ret = GetLastError();
        goto error;
    }

    // Get program name and publisher information from
    // signer info structure.
    ret = GetProgAndPublisherInfo(pSignerInfo, result);
    if (ret != 0) {
        goto error;
    }

    // Search for the signer certificate in the temporary
    // certificate store.
    CertInfo.Issuer = pSignerInfo->Issuer;
    CertInfo.SerialNumber = pSignerInfo->SerialNumber;

    pCertContext = CertFindCertificateInStore(hStore,
                                              ENCODING,
                                              0,
                                              CERT_FIND_SUBJECT_CERT,
                                              (PVOID)&CertInfo,
                                              NULL);
    if (!pCertContext) {
        ret = GetLastError();
        goto error;
    }

    result->signer_cert_serial_number =
        AllocateAndFormatSerialNumber(pCertContext);

    result->issuer_name = AllocateAndGetIssuerName(pCertContext);
    result->subject_name = AllocateAndGetSubjectName(pCertContext);

    // Get the timestamp certificate signerinfo structure.
    if (GetTimeStampSignerInfo(pSignerInfo, &pCounterSignerInfo)) {
        // Search for Timestamp certificate in the temporary
        // certificate store.
        CertInfo.Issuer = pCounterSignerInfo->Issuer;
        CertInfo.SerialNumber = pCounterSignerInfo->SerialNumber;

        pCertContext = CertFindCertificateInStore(hStore,
                                                  ENCODING,
                                                  0,
                                                  CERT_FIND_SUBJECT_CERT,
                                                              (PVOID)&CertInfo,
                                                  NULL);
        if (!pCertContext) {
            ret = GetLastError();
            goto error;
        }

        result->timestamp_issuer_name = AllocateAndGetIssuerName(pCertContext);
        result->timestamp_subject_name = AllocateAndGetSubjectName(pCertContext);

        // Find Date of timestamp.
        if (GetDateOfTimeStamp(pCounterSignerInfo, &st)) {
            char *timestamp = malloc(255);

            snprintf(timestamp, 254,
                     "%04d/%02d/%02d %02d:%02d:%02d",
                     st.wYear,
                     st.wMonth,
                     st.wDay,
                     st.wHour,
                     st.wMinute,
                     st.wSecond);

            result->timestamp = timestamp;
        }
    }

    // Now use WinVerifyTrust to make sure we actually trust it.
    result->trusted = VerifyEmbeddedSignature(filename);

 error:
    // Clean up.
    if (pSignerInfo != NULL) free(pSignerInfo);
    if (pCounterSignerInfo != NULL) free(pCounterSignerInfo);
    if (pCertContext != NULL) CertFreeCertificateContext(pCertContext);
    if (hStore != NULL) CertCloseStore(hStore, 0);
    if (hMsg != NULL) CryptMsgClose(hMsg);

    return ret;
}

LPWSTR AllocateAndCopyWideString(LPCWSTR inputString) {
    LPWSTR outputString = NULL;

    outputString = (LPWSTR)malloc(
        (wcslen(inputString) + 1) * sizeof(WCHAR));
    if (outputString != NULL) {
        lstrcpyW(outputString, inputString);
    }
    return outputString;
}

// https://code.msdn.microsoft.com/windowsdesktop/WinVerifyTrust-signature-a95ab1f6
char *VerifyEmbeddedSignature(LPCWSTR pwszSourceFile) {
    char *res = "";
    LONG lStatus;
    DWORD dwLastError;

    // Initialize the WINTRUST_FILE_INFO structure.

    WINTRUST_FILE_INFO FileData;
    memset(&FileData, 0, sizeof(FileData));
    FileData.cbStruct = sizeof(WINTRUST_FILE_INFO);
    FileData.pcwszFilePath = pwszSourceFile;
    FileData.hFile = NULL;
    FileData.pgKnownSubject = NULL;

    /*
    WVTPolicyGUID specifies the policy to apply on the file
    WINTRUST_ACTION_GENERIC_VERIFY_V2 policy checks:

    1) The certificate used to sign the file chains up to a root
    certificate located in the trusted root certificate store. This
    implies that the identity of the publisher has been verified by
    a certification authority.

    2) In cases where user interface is displayed (which this example
    does not do), WinVerifyTrust will check for whether the
    end entity certificate is stored in the trusted publisher store,
    implying that the user trusts content from this publisher.

    3) The end entity certificate has sufficient permission to sign
    code, as indicated by the presence of a code signing EKU or no
    EKU.
    */

    GUID WVTPolicyGUID = WINTRUST_ACTION_GENERIC_VERIFY_V2;
    WINTRUST_DATA WinTrustData;

    // Initialize the WinVerifyTrust input data structure.

    // Default all fields to 0.
    memset(&WinTrustData, 0, sizeof(WinTrustData));

    WinTrustData.cbStruct = sizeof(WinTrustData);

    // Use default code signing EKU.
    WinTrustData.pPolicyCallbackData = NULL;

    // No data to pass to SIP.
    WinTrustData.pSIPClientData = NULL;

    // Disable WVT UI.
    WinTrustData.dwUIChoice = WTD_UI_NONE;

    // No revocation checking.
    WinTrustData.fdwRevocationChecks = WTD_REVOKE_NONE;

    // Verify an embedded signature on a file.
    WinTrustData.dwUnionChoice = WTD_CHOICE_FILE;

    // Verify action.
    WinTrustData.dwStateAction = WTD_STATEACTION_VERIFY;

    // Verification sets this value.
    WinTrustData.hWVTStateData = NULL;

    // Not used.
    WinTrustData.pwszURLReference = NULL;

    // This is not applicable if there is no UI because it changes
    // the UI to accommodate running applications instead of
    // installing applications.
    WinTrustData.dwUIContext = 0;

    // Set pFile.
    WinTrustData.pFile = &FileData;

    // WinVerifyTrust verifies signatures as specified by the GUID
    // and Wintrust_Data.
    lStatus = WinVerifyTrust(NULL,
                             &WVTPolicyGUID,
                             &WinTrustData);

    switch (lStatus) {
    case ERROR_SUCCESS:
        /*
          Signed file:
          - Hash that represents the subject is trusted.

          - Trusted publisher without any verification errors.

          - UI was disabled in dwUIChoice. No publisher or
          time stamp chain errors.

          - UI was enabled in dwUIChoice and the user clicked
          "Yes" when asked to install and run the signed
          subject.
        */
        return "trusted";

    case TRUST_E_NOSIGNATURE:
        // The file was not signed or had a signature
        // that was not valid.

        // Get the reason for no signature.
        dwLastError = GetLastError();
        if (TRUST_E_NOSIGNATURE == dwLastError ||
            TRUST_E_SUBJECT_FORM_UNKNOWN == dwLastError ||
            TRUST_E_PROVIDER_UNKNOWN == dwLastError) {
            res = "unsigned";
        } else {
            res = "invalid signature";
        }
        break;

    case TRUST_E_EXPLICIT_DISTRUST:
        // The hash that represents the subject or the publisher
        // is not allowed by the admin or user.
        res = "disallowed";
        break;

    case TRUST_E_SUBJECT_NOT_TRUSTED:
        // The user clicked "No" when asked to install and run.
        res = "untrusted";
        break;

    case CRYPT_E_SECURITY_SETTINGS:
        /*
          The hash that represents the subject or the publisher
          was not explicitly trusted by the admin and the
          admin policy has disabled user trust. No signature,
          publisher or time stamp errors.
        */
        res = "untrusted by configuration";
        break;

    default:
        // The UI was disabled in dwUIChoice or the admin policy
        // has disabled user trust. lStatus contains the
        // publisher or time stamp chain error.
        res = "error";
        break;
    }

    // Any hWVTStateData must be released by a call with close.
    WinTrustData.dwStateAction = WTD_STATEACTION_CLOSE;

    lStatus = WinVerifyTrust(NULL,
                             &WVTPolicyGUID,
                             &WinTrustData);

    return res;
}

int GetProgAndPublisherInfo(
    PCMSG_SIGNER_INFO pSignerInfo,
    authenticode_data_struct *result) {

    int ret = 0;
    PSPC_SP_OPUS_INFO OpusInfo = NULL;
    DWORD dwData;
    BOOL fResult;

    // Loop through authenticated attributes and find
    // SPC_SP_OPUS_INFO_OBJID OID.
    for (DWORD n = 0; n < pSignerInfo->AuthAttrs.cAttr; n++) {
        if (lstrcmpA(SPC_SP_OPUS_INFO_OBJID,
                     pSignerInfo->AuthAttrs.rgAttr[n].pszObjId) == 0) {

            // Get Size of SPC_SP_OPUS_INFO structure.
            fResult = CryptDecodeObject(ENCODING,
                                        SPC_SP_OPUS_INFO_OBJID,
                                        pSignerInfo->AuthAttrs.rgAttr[n].rgValue[0].pbData,
                                        pSignerInfo->AuthAttrs.rgAttr[n].rgValue[0].cbData,
                                        0,
                                        NULL,
                                        &dwData);
            if (!fResult) {
                ret = GetLastError();
                goto error;
            }

            // Allocate memory for SPC_SP_OPUS_INFO structure.
            OpusInfo = (PSPC_SP_OPUS_INFO)malloc(dwData);
            if (!OpusInfo) {
                ret = -1;
                goto error;
            }

            // Decode and get SPC_SP_OPUS_INFO structure.
            fResult = CryptDecodeObject(ENCODING,
                                        SPC_SP_OPUS_INFO_OBJID,
                                        pSignerInfo->AuthAttrs.rgAttr[n].rgValue[0].pbData,
                                        pSignerInfo->AuthAttrs.rgAttr[n].rgValue[0].cbData,
                                        0,
                                        OpusInfo,
                                        &dwData);
            if (!fResult) {
                ret = GetLastError();
                goto error;
            }

            // Fill in Program Name if present.
            if (OpusInfo->pwszProgramName) {
                result->program_name =
                    AllocateAndCopyWideString(OpusInfo->pwszProgramName);
            }

            // Fill in Publisher Information if present.
            if (OpusInfo->pPublisherInfo) {
                switch (OpusInfo->pPublisherInfo->dwLinkChoice) {
                case SPC_URL_LINK_CHOICE:
                    result->publisher_link =
                        AllocateAndCopyWideString(OpusInfo->pPublisherInfo->pwszUrl);
                    break;

                case SPC_FILE_LINK_CHOICE:
                    result->publisher_link =
                        AllocateAndCopyWideString(OpusInfo->pPublisherInfo->pwszFile);
                    break;
                }
            }

            // Fill in More Info if present.
            if (OpusInfo->pMoreInfo) {
                switch (OpusInfo->pMoreInfo->dwLinkChoice) {
                case SPC_URL_LINK_CHOICE:
                    result->more_info_link =
                        AllocateAndCopyWideString(OpusInfo->pMoreInfo->pwszUrl);
                    break;

                case SPC_FILE_LINK_CHOICE:
                    result->more_info_link =
                        AllocateAndCopyWideString(OpusInfo->pMoreInfo->pwszFile);
                    break;
                }
            }

            break; // Break from for loop.
        } // lstrcmp SPC_SP_OPUS_INFO_OBJID
    } // for

 error:
    if (OpusInfo != NULL) free(OpusInfo);

    return ret;
}

BOOL GetDateOfTimeStamp(PCMSG_SIGNER_INFO pSignerInfo, SYSTEMTIME *st)
{
    BOOL fResult;
    FILETIME lft, ft;
    DWORD dwData;
    BOOL fReturn = FALSE;

    // Loop through authenticated attributes and find
    // szOID_RSA_signingTime OID.
    for (DWORD n = 0; n < pSignerInfo->AuthAttrs.cAttr; n++) {
        if (lstrcmpA(szOID_RSA_signingTime,
                     pSignerInfo->AuthAttrs.rgAttr[n].pszObjId) == 0) {
            // Decode and get FILETIME structure.
            dwData = sizeof(ft);
            fResult = CryptDecodeObject(ENCODING,
                                        szOID_RSA_signingTime,
                                        pSignerInfo->AuthAttrs.rgAttr[n].rgValue[0].pbData,
                                        pSignerInfo->AuthAttrs.rgAttr[n].rgValue[0].cbData,
                                        0,
                                        (PVOID)&ft,
                                        &dwData);
            if (!fResult) {
                break;
            }

            // Convert to local time.
            FileTimeToLocalFileTime(&ft, &lft);
            FileTimeToSystemTime(&lft, st);

            fReturn = TRUE;

            break; // Break from for loop.

        } //lstrcmp szOID_RSA_signingTime
    } // for

    return fReturn;
}

BOOL GetTimeStampSignerInfo(PCMSG_SIGNER_INFO pSignerInfo, PCMSG_SIGNER_INFO *pCounterSignerInfo)
{
    PCCERT_CONTEXT pCertContext = NULL;
    BOOL fReturn = FALSE;
    BOOL fResult;
    DWORD dwSize;

    *pCounterSignerInfo = NULL;

    // Loop through unathenticated attributes for
    // szOID_RSA_counterSign OID.
    for (DWORD n = 0; n < pSignerInfo->UnauthAttrs.cAttr; n++) {
        if (lstrcmpA(pSignerInfo->UnauthAttrs.rgAttr[n].pszObjId,
                     szOID_RSA_counterSign) == 0) {
            // Get size of CMSG_SIGNER_INFO structure.
            fResult = CryptDecodeObject(ENCODING,
                                        PKCS7_SIGNER_INFO,
                                        pSignerInfo->UnauthAttrs.rgAttr[n].rgValue[0].pbData,
                                        pSignerInfo->UnauthAttrs.rgAttr[n].rgValue[0].cbData,
                                        0,
                                        NULL,
                                        &dwSize);
            if (!fResult) {
                goto error;
            }

            // Allocate memory for CMSG_SIGNER_INFO.
            *pCounterSignerInfo = (PCMSG_SIGNER_INFO)malloc(dwSize);
            if (!*pCounterSignerInfo) {
                goto error;
            }

            // Decode and get CMSG_SIGNER_INFO structure
            // for timestamp certificate.
            fResult = CryptDecodeObject(ENCODING,
                                        PKCS7_SIGNER_INFO,
                                        pSignerInfo->UnauthAttrs.rgAttr[n].rgValue[0].pbData,
                                        pSignerInfo->UnauthAttrs.rgAttr[n].rgValue[0].cbData,
                                        0,
                                        (PVOID)*pCounterSignerInfo,
                                        &dwSize);
            if (!fResult) {
                goto error;
            }

            fReturn = TRUE;

            break; // Break from for loop.
        }
    }

 error:
    // Clean up.
    if (pCertContext != NULL) CertFreeCertificateContext(pCertContext);
    return fReturn;
}
