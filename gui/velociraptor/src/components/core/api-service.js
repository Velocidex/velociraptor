import axios from 'axios';

import _ from 'lodash';


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
