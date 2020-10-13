'use strict';

goog.module('grrUi.client.buildCollectorDirective');
goog.module.declareLegacyNamespace();


const BuildCollectorController = function(
    $scope, grrApiService) {

    /** @private {!angular.Scope} */
    this.scope_ = $scope;

    /** @private {!ApiService} */
    this.grrApiService_ = grrApiService;

    this.os = "Windows";
    this.names = [];
    this.params = {};
    this.tools = {
        "VelociraptorWindows": true,
        "VelociraptorLinux": true,
        "VelociraptorWindows_x86": true,
        "VelociraptorDarwin": true,
    };
    this.ops_per_second = 0;
    this.timeout = 3600;
    this.password = "";
    this.target = "ZIP";
    this.target_args = {};
    this.checking_tools = [];
    this.current_checking_tool = "";

    var self = this;
    this.template_artifacts = ["Reporting.Default"];
    this.template = "Reporting.Default";

    this.grrApiService_.get("v1/GetArtifacts", {report_type: "html"}).
        then(function(response) {
            self.template_artifacts = [];

            for(var i = 0; i<response.data.items.length; i++) {
                var item = response.data.items[i];
                self.template_artifacts.push(item["name"]);
            };
        });
};

BuildCollectorController.prototype.checkTools = function() {
    var self = this;
    var tools = Object.keys(self.tools);

    // If no tools left just make the final request.
    if (tools.length == 0) {
        self.sendRequest();
        return;
    }

    // Recursively call this function with the first tool.
    var first_tool = tools[0];

    // Clear it.
    delete self.tools[first_tool];

    // Inform the user we are checking this tool.
    self.current_checking_tool = first_tool;
    self.checking_tools.push(first_tool);

    var url = 'v1/GetToolInfo';
    var params = {
        name: first_tool,
        materialize: true,
    };
    self.grrApiService_.get("v1/GetToolInfo", params).then(function(response) {
        // Check the next tool
        self.checkTools();
    });
};

BuildCollectorController.prototype.sendRequest = function() {
    var self = this;

    var artifact_request = {
        client_id: "server",
        artifacts: ["Server.Utils.CreateCollector"],
        parameters: {
            env: [
                {key: "OS", value: this.os},
                {key: "artifacts", value: JSON.stringify(this.names)},
                {key: "parameters", value: JSON.stringify(this.params)},
                {key: "template", value: this.template},
                {key: "Password", value: this.password},
                {key: "target", value: this.target},
                {key: "target_args", value: JSON.stringify(this.target_args)},
            ],
        }
    };

    this.grrApiService_.post('v1/CollectArtifact', artifact_request)
        .then(function resolve(response) {
            this.scope_.onResolve();
        }.bind(this), function reject(response) {
            this.scope_.onReject();
        }.bind(this));
};


exports.BuildCollectorDirective = function() {
    return {
        scope: {
            onResolve: '&',
            onReject: '&',
        },
        restrict: 'E',
        templateUrl: window.base_path+'/static/angular-components/client/build-collector.html',
        controller: BuildCollectorController,
        controllerAs: 'controller',
    };
};


exports.BuildCollectorDirective.directive_name = 'grrBuildCollector';
