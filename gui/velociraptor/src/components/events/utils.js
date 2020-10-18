import _ from 'lodash';
import api from '../core/api-service.js';

function _event_table2table(event_table) {
    let result = {artifacts: event_table.artifacts,
                  parameters: {}};
    let env = event_table.parameters && event_table.parameters.env;
    _.each(env, x=>{result.parameters[x.key] = x.value;});

    return result;
}


// Convert a proto to an internal representation:
// Returns a dict with keys as labels and values as decriptors.
// A descriptor is an object with {artifacts: [], parameters: {}}
// The artifact list is a list of artifact descriptors as obtained
// from the server.
function proto2tables(table, cb) {
    let all_artifacts = [];
    let definitions = {};
    let result = {All: _event_table2table(table.artifacts)};
    all_artifacts = [...result.All.artifacts];
    _.each(table.label_events, x=>{
        let event_table = _event_table2table(x.artifacts);
        result[x.label] = event_table;
        all_artifacts = all_artifacts.concat(event_table.artifacts);
    });

    // Now lookup all the artifacts for their definitions.
    api.get("v1/GetArtifacts", {names: all_artifacts}).then(response=>{
            if (response && response.data && response.data.items) {
                _.each(response.data.items, x=>{definitions[x.name]=x;});

                // Now loop over all the artifacts and replace them
                // with the definitions.
                _.each(result, (v, k) => {
                    let artifacts = [];
                    _.each(v.artifacts, name=>{
                        if (definitions[name]) {
                            artifacts.push(definitions[name]);
                        }
                    });
                    v.artifacts = artifacts;
                });

                // Send the result.
                cb(result);
            }});

    return result;
}

function _event_table2proto(event_table) {
    if (!event_table) {
        return {artifacts: []};
    }
    let result = {artifacts: [], parameters: {env: []}};
    _.each(event_table.artifacts, def=>{
        result.artifacts.push(def.name);
    });

    _.each(event_table.parameters, (v, k) => {
        result.parameters.env.push({key: k, value: v});
    });

    return result;
}


function table2protobuf(table) {
    let label_events = [];
    let result = {artifacts: _event_table2proto(table.All)};
    _.each(table, (v, k) => {
        if (k !== "All" && !_.isEmpty(v.artifacts)) {
            label_events.push({
                label: k,
                artifacts: _event_table2proto(v)});
        }
    });

    if (!_.isEmpty(label_events)) {
        result.label_events = label_events;
    }

    return result;
}


export default {
    proto2tables: proto2tables,
    table2protobuf: table2protobuf,
};
