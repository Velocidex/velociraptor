import parse from 'csv-parse/lib/browser/sync.js';
import stringify from 'csv-stringify/lib/browser/sync.js';
import api from '../core/api-service.jsx';
import _ from 'lodash';

export function serializeCSV(data, columns) {
    let spec = [];
    _.each(columns, (k) => {
        spec.push({key: k});
    });
    return stringify(data, {columns: spec, header: true});
}


export function validateCSV(data) {
    try {
        parse(data, {skip_empty_lines: false});
        parse(data, {columns: true, skip_empty_lines: false});
        return "";

    } catch(e) {
        return e;
    }
};

export function parseCSV(data) {
    try {
        let records = parse(data, {skip_empty_lines: false});
        let obj_records = parse(data, {columns: true, skip_empty_lines: false});

        if (records && records.length > 0) {
            return {columns: records[0], data: obj_records};
        }

    } catch(e) {
        api.error("Error: " + e);
        return {columns: ["Column1"], data: []};
    }

    return {columns: ["Column1"], data: []};
}
