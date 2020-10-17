import axios from 'axios';

import _ from 'lodash';

const api_handlers = "/api/";

const get = function(url, params, cancel_token) {
    return axios({
        method: 'get',
        url: api_handlers + url,
        params: params,
        cancelToken: cancel_token,
    });
};

const get_blob = function(url, params, cancel_token) {
    return axios({
        responseType: 'blob',
        method: 'get',
        url: api_handlers + url,
        params: params,
        cancelToken: cancel_token,
    }).then((blob) => {
        return blob.data.text();
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
    });
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
    });
};


export default {
    get: get,
    get_blob: get_blob,
    post: post,
    upload: upload,
};
