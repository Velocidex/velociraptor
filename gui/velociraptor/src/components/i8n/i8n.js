import _ from 'lodash';

import English from './en.js';
import Deutsch from './de.js';

const debug = false;

function dict(item) {
    let lang = (window.globals && window.globals.lang) || "en";
    switch (lang) {
    case "en":
        return English[item];

    case "de":
        return Deutsch[item];

    default:
        return English[item];
    }
};

function T(item, ...args) {
    let lang = (window.globals && window.globals.lang) || "en";
    let d = dict(item);
    if(_.isUndefined(d)) {
        if (debug && lang !== "en") {
            let x = window.globals.dict || {};
            x[item] = item;
            window.globals["dict"] = x;
            console.log(lang, ": No translation for ", item);
        }
        d = item;
    }
    if (typeof d === 'function') {
        return d.call(null, ...args);
    }
    return d;
};

export default T;
