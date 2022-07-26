import axios from 'axios';

import _ from 'lodash';

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
  return error.code !== 'ECONNABORTED' && (!error.response || (
      error.response.status >= 500 && error.response.status <= 599));
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

let base_path = window.base_path || "";

// In development we only support running from /
if (!process.env.NODE_ENV || process.env.NODE_ENV === 'development') {
    base_path = "";
}

let api_handlers = base_path + "/api/";

const handle_error = err=>{
    if (axios.isCancel(err)) {
        return {data: {}, cancel: true};
    };

    let data = err.response && err.response.data;
    data = data || err.message;

    if (data instanceof Blob) {
        return data.text();
    } else if(data.message) {
        data = data.message;
    };

    // Call all the registered hooks.
    _.each(hooks, h=>h("Error: " + data));
    throw err;
};


const get = function(url, params, cancel_token) {
    return axios({
        method: 'get',
        url: api_handlers + url,
        params: params,
        headers: {
            "X-CSRF-Token": window.CsrfToken,
            "Grpc-Metadata-OrgId": window.globals.OrgId || "",
        },
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
        url: api_handlers + url,
        params: params,
        headers: {
            "X-CSRF-Token": window.CsrfToken,
            "Grpc-Metadata-OrgId": window.globals.OrgId || "",
        },
        cancelToken: cancel_token,
    }).then((blob) => {
        var arrayPromise = new Promise(function(resolve) {
            var reader = new FileReader();

            reader.onloadend = function() {
                resolve(reader.result);
            };

            reader.readAsArrayBuffer(blob.data);
        });

        return arrayPromise;
    }).catch(err=>{
        let data = err.response && err.response.data;
        data.text().then((message)=>_.each(hooks, h=>h("Error: " + message)));
        return "";
    });
};

const post = function(url, params, cancel_token) {
    return axios({
        method: 'post',
        url: api_handlers + url,
        data: params,
        cancelToken: cancel_token,
        headers: {
            "X-CSRF-Token": window.CsrfToken,
            "Grpc-Metadata-OrgId": window.globals.OrgId || "",
        }
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
        url: api_handlers + url,
        data: fd,
        headers: {
            "X-CSRF-Token": window.CsrfToken,
            "Grpc-Metadata-OrgId": window.globals.OrgId || "",
        }
    }).catch(handle_error);
};

var hooks = [];

/* eslint import/no-anonymous-default-export: [2, {"allowObject": true}] */
export default {
    get: get,
    get_blob: get_blob,
    post: post,
    upload: upload,
    hooks: hooks,
    base_path: base_path,
};
