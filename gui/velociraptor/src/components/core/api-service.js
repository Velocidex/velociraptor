import axios from 'axios';

const get = function(url, params) {
    return axios({
        method: 'get',
        url: url,
        params: params,
    });
};

export default {
    get: get,
};
