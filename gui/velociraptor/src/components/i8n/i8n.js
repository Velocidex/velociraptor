import _ from 'lodash';

import Deutsch from './de.js';
import English from './en.js';
import French from './fr.js';
import Japanese from './jp.js';
import Portuguese from './por.js';
import Spanish from './es.js';


const debug = false;

function dict(item) {
    let lang = (window.globals && window.globals.lang) || "en";
    switch (lang) {
    case "en":
        return English[item];

    case "es":
        return Spanish[item];

    case "por":
        return Portuguese[item];

    case "de":
        return Deutsch[item];

    case "fr":
        return French[item];

    case "jp":
        return Japanese[item];

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
