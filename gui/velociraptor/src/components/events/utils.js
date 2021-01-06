import _ from 'lodash';
import api from '../core/api-service.js';

// Convert flows_proto.ArtifactParameters to a k,v dict.
function _ArtifactParameters2dict(parameters) {
    let result = {};
    let env = parameters && parameters.env;
    _.each(env, x=>{result[x.key] = x.value;});
    return result;
};

function _ArtifactCollectorArgs_to_label_table(event_table) {
    let result = {
        // Will be replaced by full definition later.
        artifacts: event_table.artifacts,
        specs: {},
    };

    _.each(event_table.specs, spec=>{
        let artifact_name = spec.artifact;
        result.specs[artifact_name] = _ArtifactParameters2dict(spec.parameters);
    });

    // If there is no spec the add an empty one.
    _.each(result.artifacts, artifact_name=>{
        if(_.isUndefined(result.specs[artifact_name])) {
            result.specs[artifact_name] = {};
        }
    });

    return result;
}


// Convert a flows_proto.ClientEventTable proto to an internal
// client_event_table representation:
/*
   client_event_table := {
     "All": label_table,    <-- applies to all clients
     "label1": label_table,   <-- applies to clients labeled as label1
   }

   label_table := {
     "artifacts": artifact_definition_list,   <-- Full artifact definitions
     "specs": parameter_spec,
   }

   parameter_spec := {
        artifact_name: {
             key: value,
             key2, value2,
        },
   }
*/
function proto2tables(table, cb) {
    let definitions = {};
    let result = {All: _ArtifactCollectorArgs_to_label_table(table.artifacts)};
    let all_artifacts = [...result.All.artifacts];

    _.each(table.label_events, x=>{
        let event_table = _ArtifactCollectorArgs_to_label_table(x.artifacts);
        result[x.label] = event_table;
        all_artifacts = all_artifacts.concat(event_table.artifacts);
    });

    // Now lookup all the artifacts for their definitions and replace
    // in client_event_table.
    api.get("v1/GetArtifacts", {names: all_artifacts}).then(response=>{
        if (response && response.data && response.data.items) {
            // Create a big lookup table for all artifacts and their
            // definitions.
            _.each(response.data.items, x=>{
                definitions[x.name] = x;
            });

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

function _label_table2ArtifactCollectorArgs(label_table) {
    let result = {artifacts: [], specs: []};

    if (!label_table) {
        return result;
    }

    _.each(label_table.artifacts, def=>{
        result.artifacts.push(def.name);
    });

    _.each(label_table.specs, (v, k) => {
        // Skip specs for artifacts that are not present.
        if(!result.artifacts.includes(k)) {
            return;
        }

        let artifact_name = k;
        let parameters = {env: []};
        _.each(v, (v, k)=>{
            parameters.env.push({key: k, value: v});
        });

        result.specs.push({artifact: artifact_name, parameters: parameters});
    });

    return result;
}


function table2protobuf(table) {
    let label_events = [];
    let result = {artifacts: _label_table2ArtifactCollectorArgs(table.All)};
    _.each(table, (v, k) => {
        if (k !== "All" && !_.isEmpty(v.artifacts)) {
            label_events.push({
                label: k,
                artifacts: _label_table2ArtifactCollectorArgs(v)});
        }
    });

    if (!_.isEmpty(label_events)) {
        result.label_events = label_events;
    }

    return result;
}


/* eslint import/no-anonymous-default-export: [2, {"allowObject": true}] */
export default {
    proto2tables: proto2tables,
    table2protobuf: table2protobuf,
};
