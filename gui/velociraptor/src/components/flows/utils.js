import _ from 'lodash';
import api from '../core/api-service.js';


// returns a dict with keys: artifact name and values are a dict with
// k,v pairs for the named artifact parameters
const requestToParameters = (request) => {
    let parameters = {};

    // New style request has specs field.
    if (!_.isEmpty(request.specs)) {
        let parameters = {};
        _.each(request.specs, spec=>{
            let artifact_parameters = {};
            _.each(spec.parameters.env, param=>{
                artifact_parameters[param.key] = param.value;
            });
            parameters[spec.artifact] = artifact_parameters;
        });

        return parameters;
    }

    // Old style request has artifacts + shared parameters fields.
    if (!_.isEmpty(request.parameters)) {
        _.each(request.artifacts, name=>{
            _.each(request.parameters.env, param=>{
                if(_.isUndefined(parameters[name])) {
                    parameters[name] = {};
                };
                parameters[name][param.key] = param.value;
            });
        });
    }

    return parameters;
};

const POLL_TIME = 1000;

const runArtifact = (client_id, artifact, params, on_success, token)=>{
    let artifact_params = {env: []};
    _.each(params, (v, k) => {
        artifact_params.env.push({key: k, value: v});
    });

    api.post("v1/CollectArtifact", {
        client_id: client_id,
        artifacts: [artifact],
        specs: [{artifact: artifact,
                 parameters: artifact_params}],
        max_upload_bytes: 1048576000,
    }, token).then((response) => {
        let flow_id = response.data.flow_id;

        // Start polling for flow completion.
        let recursive_download_interval = setInterval(() => {
            api.get("v1/GetFlowDetails", {
                client_id: client_id,
                flow_id: flow_id,
            }, token).then((response) => {
                let context = response.data.context || {};
                if (context.state === "RUNNING") {
                    return;
                }

                // The node is refreshed with the correct flow id, we can stop polling.
                clearInterval(recursive_download_interval);
                on_success(context);
            });
        }, POLL_TIME);
    });
};


export {
    requestToParameters,
    runArtifact,
}
