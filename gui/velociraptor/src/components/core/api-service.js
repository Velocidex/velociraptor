import axios from 'axios';

const get = function(url, params, cancel_token) {
    return axios({
        method: 'get',
        url: url,
        params: params,
        cancelToken: cancel_token,
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


export default {
    get: get,
    post: post,
};
