import _ from 'lodash';
import { JSONparse } from '../utils/json_parse.jsx';

// Parse the response from GetTable into a list of objects.
export function parseTableResponse(response) {
    let data = [];
    let columns = response.data.columns || [];
    for(var row=0; row<response.data.rows.length; row++) {
        let item = {};
        let current_row = JSONparse(response.data.rows[row].json);
        if (!_.isArray(current_row)) {
            continue;
        }

        if(current_row.length > columns.length) {
            current_row = current_row.splice(columns.length);
        }

        for(let column=0; column<current_row.length; column++) {
            item[columns[column]] = current_row[column];
        }
        data.push(item);
    }
    return data;
};
