// +build windows

// References:
// https://www.codeproject.com/Articles/10539/Making-WMI-Queries-In-C
// https://www.codeproject.com/Articles/13601/COM-in-plain-C

#define _WIN32_DCOM
#include <stdint.h>
#include <wbemidl.h>
#include <initguid.h>
#include <stdio.h>

// The Go function which will receive the encoded event object.
void process_event(void *ctx, short unsigned int **bstring);
void log_error(void *c_ctx, char *message);
void Error(void *go_ctx, char *function, HRESULT hres);

BSTR UTF8ToBStr(char *str) {
    int wchars_num =  MultiByteToWideChar(CP_UTF8 , 0 , str , -1, NULL , 0 );
    wchar_t* wstr = (wchar_t*)calloc(wchars_num, sizeof(wchar_t));
    MultiByteToWideChar(CP_UTF8 , 0 , str , -1, wstr , wchars_num );
    BSTR bstr = SysAllocString(wstr);
    free(wstr);

    return bstr;
}



typedef struct {
    const IWbemEventSinkVtbl *lpVtbl;

    LONG refcount;

    // The Go object we call with each event.
    void *ctx;

} EventSink;

HRESULT STDMETHODCALLTYPE QueryInterface(
    IWbemEventSink *self,
    const IID *riid, void **ppv){
    if ( IsEqualIID(riid, &IID_IUnknown) ||
         IsEqualIID(riid, &IID_IWbemObjectSink) ) {
        *ppv = (IWbemObjectSink *) self;
        self->lpVtbl->AddRef(self);
        return WBEM_S_NO_ERROR;
    } else return E_NOINTERFACE;
}

ULONG STDMETHODCALLTYPE AddRef(IWbemEventSink *This)
{
    EventSink *self = (EventSink *)This;
    return InterlockedIncrement(&self->refcount);
}

ULONG STDMETHODCALLTYPE Release(IWbemEventSink *This) {
    EventSink *self = (EventSink *)This;
    LONG lRef = InterlockedDecrement(&self->refcount);
    if(lRef == 0) {
        GlobalFree(self);
        return 0;
    }

    return lRef;
}

HRESULT Indicate(IWbemEventSink *This,
                 LONG ObjectCount,
                 IWbemClassObject **apObjArray) {
    EventSink *self = (EventSink *)This;
    int i;
    for (i = 0; i < ObjectCount; i++) {
        BSTR pstrObjectText = NULL;
        IWbemClassObject *pObj = apObjArray[i];

        HRESULT hr = pObj->lpVtbl->GetObjectText(pObj, 0, &pstrObjectText);
        if (SUCCEEDED(hr)) {
            // Call into the Go runtime with this new event. We just
            // serialize the event into text and pass the text to Go.

            // It is a bit hacky but it simplies the code a lot and
            // mostly does what we want without doing the whole COM
            // dance. If we ever need to actually call methods on the
            // objects (for example terminate a process), we can deal
            // with it later.
            process_event(self->ctx, &pstrObjectText);
            SysFreeString(pstrObjectText);
        }
    }
    return WBEM_S_NO_ERROR;
}

HRESULT SetStatus(
    IWbemEventSink *This,
    /* [in] */ LONG lFlags,
    /* [in] */ HRESULT hResult,
    /* [in] */ BSTR strParam,
    /* [in] */ IWbemClassObject __RPC_FAR *pObjParam) {
    return WBEM_S_NO_ERROR;
}

static const IWbemEventSinkVtbl EventSink_Vtbl = {
    QueryInterface,
    AddRef,
    Release,
    Indicate,
    SetStatus
};


EventSink *NewEventSink() {
    EventSink *result = (EventSink *)GlobalAlloc(
        GMEM_FIXED, sizeof(EventSink));
    result->lpVtbl = &EventSink_Vtbl;
    result->refcount = 0;

    return result;
}

// Keeps the WMI context so we can clean up properly.
typedef struct  {
    IWbemServices *service;
    IWbemLocator *locator;
    IUnsecuredApartment* apartment;
    IUnknown* unknown_stub;
    EventSink* sink;
    IWbemObjectSink* stub_sink;
} watcher_context;

// These functions are called from Go to create and destroy an event
// watcher context.
void *watchEvents(void *go_ctx, char *query, char* namespace);
void destroyEvent(void *c_ctx);
void log_error(void *go_ctx, char *message);

// Allocate and initialize an event watcher context.  Returns the
// context of NULL on error. The go_ctx is an opaque Go pointer which
// will be passed into the go callback. NOTE: This must be allocated
// using the pointer package's pointer.Save().
void *watchEvents(void *go_ctx, char *query, char *namespace) {
    HRESULT hres;
    watcher_context *ctx = (watcher_context *)calloc(sizeof(watcher_context), 1);

    if (ctx == NULL) {
        return NULL;
    }

    // Step 1: --------------------------------------------------
    // Initialize COM. ------------------------------------------
    hres =  CoInitializeEx(0, COINIT_MULTITHREADED);
    if (FAILED(hres)) {
        log_error(go_ctx, "Failed to initialize COM library - CoInitializeEx.");
        return NULL;
    }

    // Step 2: --------------------------------------------------
    // Set general COM security levels --------------------------
    hres =  CoInitializeSecurity(
        NULL,
        -1,                          // COM negotiates service
        NULL,                        // Authentication services
        NULL,                        // Reserved
        RPC_C_AUTHN_LEVEL_DEFAULT,   // Default authentication
        RPC_C_IMP_LEVEL_IMPERSONATE, // Default Impersonation
        NULL,                        // Authentication info
        EOAC_NONE,                   // Additional capabilities
        NULL                         // Reserved
    );

    // Calling CoInitializeSecurity more than once is an error we can ignore.
    if (FAILED(hres) && hres != RPC_E_TOO_LATE) {
        Error(go_ctx, "Failed to initialize COM library - CoInitializeSecurity.", hres);
        goto error;
    }

    // Step 3: ---------------------------------------------------
    // Obtain the initial locator to WMI -------------------------
    hres = CoCreateInstance(
        &CLSID_WbemLocator,
        0,
        CLSCTX_INPROC_SERVER,
        &IID_IWbemLocator, (LPVOID *) &ctx->locator);

    if (FAILED(hres)) {
        Error(go_ctx, "Failed to initialize COM library - CoCreateInstance.", hres);
        goto error;
    }

    // Connect to the local root\cimv2 namespace
    // and obtain pointer service to make IWbemServices calls.
    BSTR bstr_root = UTF8ToBStr(namespace);
    hres = ctx->locator->lpVtbl->ConnectServer(
        ctx->locator,
        bstr_root,
        NULL,
        NULL,
        0,
        0,
        0,
        0,
        &ctx->service
    );
    SysFreeString(bstr_root);

    if (FAILED(hres)) {
        Error(go_ctx, "Failed to initialize COM library - ConnectServer.", hres);
        goto error;
    }

    // Step 5: --------------------------------------------------
    // Set security levels on the proxy -------------------------
    hres = CoSetProxyBlanket(
        (IUnknown *)ctx->service,            // Indicates the proxy to set
        RPC_C_AUTHN_WINNT,           // RPC_C_AUTHN_xxx
        RPC_C_AUTHZ_NONE,            // RPC_C_AUTHZ_xxx
        NULL,                        // Server principal name
        RPC_C_AUTHN_LEVEL_CALL,      // RPC_C_AUTHN_LEVEL_xxx
        RPC_C_IMP_LEVEL_IMPERSONATE, // RPC_C_IMP_LEVEL_xxx
        NULL,                        // client identity
        EOAC_NONE                    // proxy capabilities
    );

    if (FAILED(hres)) {
        Error(go_ctx, "Failed to initialize COM library - CoSetProxyBlanket.", hres);
        goto error;
    }

    // Step 6: -------------------------------------------------
    // Receive event notifications -----------------------------

    // Use an unsecured apartment for security
    hres = CoCreateInstance(&CLSID_UnsecuredApartment, NULL,
                            CLSCTX_LOCAL_SERVER, &IID_IUnsecuredApartment,
                            (void**)&ctx->apartment);
    if (FAILED(hres)) {
        Error(go_ctx, "Failed to create IWbemLocator.", hres);
        goto error;
    }

    ctx->sink = NewEventSink();
    ctx->sink->lpVtbl->AddRef((IWbemEventSink *)ctx->sink);
    // Pass the go context along.
    ctx->sink->ctx = go_ctx;

    hres = ctx->apartment->lpVtbl->CreateObjectStub(
        ctx->apartment, (IUnknown *)ctx->sink, &ctx->unknown_stub);
    if (FAILED(hres)) {
        Error(go_ctx, "Failed to create IWbemLocator.", hres);
        goto error;
    }

    ctx->unknown_stub->lpVtbl->QueryInterface(
        ctx->unknown_stub,
        &IID_IWbemObjectSink,
        (void **) &ctx->stub_sink);

    // The ExecNotificationQueryAsync method will call
    // The EventQuery::Indicate method when an event occurs
    BSTR bstr_wql = SysAllocString(L"WQL" );

    // Convert query from UTF8 char* to bstr
    BSTR bstr_sql = UTF8ToBStr(query);

    hres = ctx->service->lpVtbl->ExecNotificationQueryAsync(
        ctx->service,
        bstr_wql, bstr_sql,
        WBEM_FLAG_SEND_STATUS,
        NULL,
        ctx->stub_sink);

    SysFreeString(bstr_wql);
    SysFreeString(bstr_sql);

    // Check for errors.
    if (FAILED(hres)) {
        Error(go_ctx, "ExecNotificationQueryAsync", hres);
        goto error;
    }
    return ctx;

 error:
    destroyEvent(ctx);
    return NULL;
}

// Destroy the C context.
void destroyEvent(void *c_ctx) {
    watcher_context *ctx = (watcher_context *)c_ctx;

    if (ctx->service) {
        ctx->service->lpVtbl->CancelAsyncCall(ctx->service, ctx->stub_sink);
        ctx->service->lpVtbl->Release(ctx->service);
    }

    if (ctx->locator) {
        ctx->locator->lpVtbl->Release(ctx->locator);
    }

    if (ctx->apartment) {
        ctx->apartment->lpVtbl->Release(ctx->apartment);
    }

    if (ctx->unknown_stub) {
        ctx->unknown_stub->lpVtbl->Release(ctx->unknown_stub);
    }

    if (ctx->sink) {
        ctx->sink->lpVtbl->Release((IWbemEventSink *)ctx->sink);
    }

    if (ctx->stub_sink) {
        ctx->stub_sink->lpVtbl->Release(ctx->stub_sink);
    }

    CoUninitialize();
    free(ctx);
}


void Error(void *go_ctx, char *function, HRESULT hres) {
    // Unfortunately its hard to retrieve the error message so we just
    // store the HRESULT - users can lookup the MSDN to figure out
    // what it means.
    char buf[512];
    snprintf(buf, 512, "%s: Error code %#lx", function, (uint32_t)hres);
    log_error(go_ctx, buf);
}
