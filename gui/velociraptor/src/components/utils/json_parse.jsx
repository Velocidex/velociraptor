import _ from 'lodash';


// JSON.parse is unsafe because it raises an exception - this
// function wraps it with a possible default value but does not raise.
export function JSONparse(x, default_value) {
    try {
        return JSON.parse(x);
    } catch(e) {
        if (!_.isUndefined(default_value)) {
            return default_value;
        }
        return x;
    };
}
