import _ from 'lodash';

import Deutsch from './de.jsx';
import English from './en.jsx';
import French from './fr.jsx';
import Japanese from './jp.jsx';
import Portuguese from './por.jsx';
import Spanish from './es.jsx';
import Vietnamese from './vi.jsx';

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

    case "vi":
        return Vietnamese[item];

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

        // If there is no specific translation in the target language
        // we use the English one, and failing that the name of the
        // item directly.
        d = English[item] || item;
    }
    if (typeof d === 'function') {
        return d.call(null, ...args);
    }
    return d;
};

export default T;
