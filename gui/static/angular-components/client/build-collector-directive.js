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
    this.ops_per_second = 0;
    this.timeout = 3600;
    this.password = "";
    this.target = "ZIP";
    this.target_args = {};

    var self = this;
    self.inventory = [];
    this.inventoryModel = [];
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
        templateUrl: '/static/angular-components/client/build-collector.html',
        controller: BuildCollectorController,
        controllerAs: 'controller',
    };
};


exports.BuildCollectorDirective.directive_name = 'grrBuildCollector';
