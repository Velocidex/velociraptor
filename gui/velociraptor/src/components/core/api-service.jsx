import axios, {isCancel} from 'axios';
import path from 'path-browserify';

import _ from 'lodash';
import qs from 'qs';
import axiosRetry from 'axios-retry';

// The following is copied from axios-retry to avoid a bug "cannot
// read properties of undefined (reading 'request')"
const denyList = new Set([
        'ENOTFOUND',
        'ENETUNREACH',

        // SSL errors from
        // https://github.com/nodejs/node/blob/fc8e3e2cdc521978351de257030db0076d79e0ab/src/crypto/crypto_common.cc#L301-L328
        'UNABLE_TO_GET_ISSUER_CERT',
        'UNABLE_TO_GET_CRL',
        'UNABLE_TO_DECRYPT_CERT_SIGNATURE',
        'UNABLE_TO_DECRYPT_CRL_SIGNATURE',
        'UNABLE_TO_DECODE_ISSUER_PUBLIC_KEY',
        'CERT_SIGNATURE_FAILURE',
        'CRL_SIGNATURE_FAILURE',
        'CERT_NOT_YET_VALID',
        'CERT_HAS_EXPIRED',
        'CRL_NOT_YET_VALID',
        'CRL_HAS_EXPIRED',
        'ERROR_IN_CERT_NOT_BEFORE_FIELD',
        'ERROR_IN_CERT_NOT_AFTER_FIELD',
        'ERROR_IN_CRL_LAST_UPDATE_FIELD',
        'ERROR_IN_CRL_NEXT_UPDATE_FIELD',
        'OUT_OF_MEM',
        'DEPTH_ZERO_SELF_SIGNED_CERT',
        'SELF_SIGNED_CERT_IN_CHAIN',
        'UNABLE_TO_GET_ISSUER_CERT_LOCALLY',
        'UNABLE_TO_VERIFY_LEAF_SIGNATURE',
        'CERT_CHAIN_TOO_LONG',
        'CERT_REVOKED',
        'INVALID_CA',
        'PATH_LENGTH_EXCEEDED',
        'INVALID_PURPOSE',
        'CERT_UNTRUSTED',
        'CERT_REJECTED',
        'HOSTNAME_MISMATCH'
]);

const _isRetryAllowed = error=> !denyList.has(error && error.code);

// https://github.com/softonic/axios-retry/issues/87
function retryDelay(retryNumber = 0) {
    const delay = Math.pow(2, retryNumber) * 1000;
    const randomSum = delay * 0.2 * Math.random(); // 0-20% of the delay
    console.log("retrying API call in " + (delay + randomSum));
    return delay + randomSum;
}

function isRetryableError(error) {
    if (error.code === 'ECONNABORTED') {
        return false;
    }

    if (!error.response) {
        return true;
    }

    // This represents a real server error no need to retry it.
    if (error.response.data && error.response.data.code) {
        return false;
    }

    if (error.response.status >= 500 && error.response.status <= 599) {
        return true;
    }

    return false;
}

function isNetworkError(error) {
  return !error.response && Boolean(error.code) && // Prevents retrying cancelled requests
  error.code !== 'ECONNABORTED' && // Prevents retrying timed out requests
  _isRetryAllowed(error); // Prevents retrying unsafe errors
}

var IDEMPOTENT_HTTP_METHODS = ['get', 'head', 'options', 'put', 'delete'];

function isIdempotentRequestError(error) {
  if (!error.config) {
      // Cannot determine if the request can be retried
      return false;
  }

  return isRetryableError(error) && IDEMPOTENT_HTTP_METHODS.indexOf(error.config.method) !== -1;
}

function isNetworkOrIdempotentRequestError(error) {
  return isNetworkError(error) || isIdempotentRequestError(error);
}

// Retry simple network errors
function simpleNetworkErrorCheck(error) {
  if ((error && error.message) === 'Network Error') {
    return true;

  } else {
      return isNetworkOrIdempotentRequestError(error);
  }
}

axiosRetry(axios, {
  retries: 3,
  retryDelay,

  retryCondition: simpleNetworkErrorCheck,
});

const base_path = ()=>{
    let base_path = window.base_path || "";
    if (base_path === "") {
        let pname = window.location.pathname;
        base_path = pname.replace(/\/app.*$/, "");
    }

    // In development we only support running from /
    if (!process.env.NODE_ENV || process.env.NODE_ENV === 'development') {
        base_path = "";
    }

    return base_path;
};



// Build the full URL to the API handlers based on the page URL.
// Example: api_handlers("/v1/foo") -> https://www.example.com/base_path/api/v1/foo
const api_handlers = url=>{
    let parsed =  new URL(window.location);
    parsed.pathname = path.join(base_path(), "api/" , url);
    parsed.hash = '';
    parsed.search = '';

    return parsed.href;
};

const handle_error = err=>{
    if (isCancel(err)) {
        return {data: {}, cancel: true};
    };

    if (err.response && err.response.status === 401) {
        const redirectTemplate = window.globals.AuthRedirectTemplate || "";
        if (redirectTemplate !== "") {
            const instantiatedTemplate = redirectTemplate.replaceAll(
                '%LOCATION%', encodeURIComponent(window.location.href));
            window.location.assign(instantiatedTemplate);
            return {data: {}, cancel: false};
        }
    }

    let data = err.response && err.response.data;
    data = data || err.message;

    const contentType = err.response && err.response.headers &&
          err.response.headers["content-type"];
    if (data instanceof Blob) {
        return data.text();
    } else if(data.message) {
        data = data.message;
    } else if(contentType === "text/html") {
      data = "";
    }

    // Call all the registered hooks.
    _.each(hooks, h=>h("Error: " + data));
    throw err;
};

const get_headers = ()=>{
    let org_id = window.globals.OrgId;
    if (org_id.substring(0,2) === "{{") {
        org_id = "";
    }

    let headers = {
        "Grpc-Metadata-OrgId": org_id || "root",
    };

    let csrf = window.CsrfToken;
    if (csrf.substring(0,2) !== "{{") {
        headers["X-CSRF-Token"] = csrf;
    };

    return headers;
};


const get = function(url, params, cancel_token) {
    return axios({
        method: 'get',
        url: api_handlers(url),
        params: params,
        headers: get_headers(),
        cancelToken: cancel_token,
    }).then(response=>{
        // Update the csrf token.
        let token = response.headers["x-csrf-token"];
        if (token && token.length > 0) {
            window.CsrfToken = token;
        }
        return response;
    }).catch(handle_error);
};

const get_blob = function(url, params, cancel_token) {
    return axios({
        responseType: 'blob',
        method: 'get',
        url: api_handlers(url),
        params: params,
        paramsSerializer: params => {
            return qs.stringify(params, {indices: false});
        },
        headers: get_headers(),
        cancelToken: cancel_token,
    }).then((blob) => {
        var arrayPromise = new Promise(function(resolve) {
            var reader = new FileReader();

            reader.onloadend = function() {
                resolve({data: reader.result, blob: blob});
            };

            reader.readAsArrayBuffer(blob.data);
        });

        return arrayPromise;
    }).catch(err=>{
        return {
            error: err,
            // If callers want to actually report the error they need
            // to call this.
            report_error: ()=>{
                let data = err.response && err.response.data;
                if(data) {
                    data.text().then((message)=>_.each(
                        hooks, h=>h("Error: " + message)));
                }
            }
        };
    });
};

const post = function(url, params, cancel_token) {
    return axios({
        method: 'post',
        url: api_handlers(url),
        data: params,
        cancelToken: cancel_token,
        headers: get_headers(),
    }).then(response=>{
        // Update the csrf token.
        let token = response.headers["x-csrf-token"];
        if (token && token.length > 0) {
            window.CsrfToken = token;
        }
        return response;
    }).catch(handle_error);
};

const upload = function(url, files, params) {
    var fd = new FormData();
    _.each(files, (v, k) => {
        fd.append(k, v);
    });

    fd.append("_params_", JSON.stringify(params || {}));

    return axios({
        method: 'post',
        url: api_handlers(url),
        data: fd,
        headers: get_headers(),
    }).catch(handle_error);
};

// Internal Routes declared in api/proxy.go Assume base_path is regex
// safe due to the sanitation in the sanitation service.
// A link is considered internal if:
// * it is relative
// * it has a known prefix and
// * it either starts with base path or not - URLs that do not start
//   with the base path will be fixed later.
const api_regex = new RegExp("^/(api|app|notebooks|downloads|hunts|clients|auth)/");

// Only recognize some urls as a valid internal link.
const internal_links = url_path=>{
    // If the url starts with the base path, then strip it before we
    // do the check.
    let base = base_path();
    if (url_path.startsWith(base)) {
        url_path = url_path.slice(base.length);

        // The URL must be absolute and rooted at /
        if (!url_path.startsWith("/")) {
            url_path = "/" + url_path;
        }
    }
    return api_regex.test(url_path);
};

// Prepare a suitable href link for <a>
// This function accepts a number of options:

// - internal: This option means the link is an internl link to
//   another part of the SPA. The function will update the path and
//   host parts of the URL to point at the current page. This is
//   useful because the caller does not need to know where the
//   application is served from.

// NOTE: Relative URLs will be converted to absolute URLs based
// on the current location so they can be bookmarked or shared.
// The org id is automatically added if needed to ensure the URLs
// refer to the correct org.
const href = function(url, params, options) {
    if (_.isEmpty(url)) {
        return window.location;
    }

    let parsed = parse_url(url);

    // The params form the query string (after ? and before #)
    params = Object.assign(parsed.params, params || {});

    // Relative URLs are always internal.
    if(url.startsWith("/") || (options && options.internal)) {
        if (_.isEmpty(params.org_id)) {
            params.org_id = window.globals.OrgId || "root";
        }

        // All internal links must point to the same page since this
        // is a SPA
        if (internal_links(parsed.pathname)) {
            parsed.pathname = src_of(parsed.pathname);
        } else {
            parsed.pathname = window.location.pathname;
        }
    }

    // Options control the type of encoding.
    options = options || {};
    Object.assign(options, {indices: false});

    parsed.search = qs.stringify(params, options);

    return parsed.href;
};

const delete_req = function(url, params, cancel_token) {
    return axios({
        method: 'delete',
        url: api_handlers(url),
        params: params,
        cancelToken: cancel_token,
        headers: get_headers(),
    }).then(response=>{
        // Update the csrf token.
        let token = response.headers["x-csrf-token"];
        if (token && token.length > 0) {
            window.CsrfToken = token;
        }
        return response;
    }).catch(handle_error);
};

var hooks = [];

// Returns a url corrected for base_path. Handles data URLs properly.
const src_of = function (url) {
    if (url && url.match(/^data/)) {
        return url;
    }

    // If the URL does not already start with base path ensure it does
    // now.
    let base = base_path();
    if (!url.startsWith(base)) {
        return path.join(base, url);
    }
    return url;
};

const error = function(msg) {
    _.each(hooks, h=>h(msg));
};

/* eslint import/no-anonymous-default-export: [2, {"allowObject": true}] */
export default {
    get: get,
    get_blob: get_blob,
    post: post,
    upload: upload,
    hooks: hooks,
    base_path: base_path,
    href: href,
    error: error,
    delete_req: delete_req,
    src_of: src_of,
};


function parse_url(url) {
    if (_.isObject(url)) {
        return url;
    }

    if (_.isString(url)) {
        try {
            let parsed =  new URL(url, window.location.origin);
            if(!_.isEmpty(parsed.search) && parsed.search[0] == "?") {
                parsed.params = qs.parse(parsed.search.substr(1));
            } else {
                parsed.params = {};
            }

            return parsed;

        } catch(e) {
            return {};
        }
    }

    return {};
};
