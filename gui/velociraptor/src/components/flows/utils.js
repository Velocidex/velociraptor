import _ from 'lodash';

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


export {
    requestToParameters,
}
