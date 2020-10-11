import axios from 'axios';

import _ from 'lodash';

const get = function(url, params, cancel_token) {
    return axios({
        method: 'get',
        url: url,
        params: params,
        cancelToken: cancel_token,
    });
};

const get_blob = function(url, params, cancel_token) {
    return axios({
        responseType: 'blob',
        method: 'get',
        url: url,
        params: params,
        cancelToken: cancel_token,
    }).then((blob) => {
        return blob.data.text();
    });
};

const post = function(url, params, cancel_token) {
    return axios({
        method: 'post',
        url: url,
        data: params,
        cancelToken: cancel_token,
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
        url: url,
        data: fd,
    });
};


export default {
    get: get,
    get_blob: get_blob,
    post: post,
    upload: upload,
};
