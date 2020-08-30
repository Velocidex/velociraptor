'use strict';

goog.module('grrUi.flow.newArtifactCollectionDirective');

const {ApiService} = goog.require('grrUi.core.apiService');


/**
 * Controller for NewArtifactCollectionController.
 *
 * @constructor
 * @param {!angular.Scope} $scope
 * @param {!ApiService} grrApiService
 * @ngInject
 */
const NewArtifactCollectionController = function(
    $scope, grrApiService) {

    /** @private {!angular.Scope} */
    this.scope_ = $scope;

    /** @private {!ApiService} */
    this.grrApiService_ = grrApiService;

    /** @type {boolean} */
    this.requestSent = false;

    /** @type {?string} */
    this.responseError;

    /** @type {?string} */
    this.responseData;

    // This controls which type of artifact we are allowed to search
    // for.
    var client_id = this.scope_['clientId'];
    if (client_id[0] == "C") {
      this.artifactType = "CLIENT";
    } else {
        this.artifactType = "SERVER";
    }

    // Configured parameters to collect the artifacts with.
    this.params = {};

    this.tools = {};
    this.checking_tools = [];
    this.current_checking_tool = "";

    // The names of the artifacts to collect.
    this.names = [];
    this.ops_per_second = 0;
    this.timeout = 600;
    this.max_rows = 1000000;
    this.max_bytes = 1000; // 1 GB

    this.flow_details = {};

    this.scope_.$watchGroup(['clientId', 'flowId'],
                            this.onClientIdChange_.bind(this));
};


NewArtifactCollectionController.prototype.onClientIdChange_ = function() {
    var url = 'v1/GetFlowDetails';
    var param = {flow_id: this.scope_["flowId"],
                 client_id: this.scope_["clientId"]};
    var self = this;

    this.grrApiService_.get(url, param).then(
        function success(response) {
            self.flow_details = response.data["context"];
            self.names = self.flow_details.request.artifacts;
            self.params = {};

            var request = self.flow_details.request || {};

            self.max_rows = request.max_rows || 1000000;
            self.timeout = request.timeout || 600;
            self.max_bytes = (request.max_upload_bytes || 0) / 1024 / 1024;

            var env = self.flow_details.request.parameters.env;
            for (var i=0; i<env.length;i++) {
                var key = env[i]["key"];
                var value = env[i]["value"];
                if (angular.isString(key)) {
                    self.params[key] = value;
                }
            }
        }.bind(this));
};

NewArtifactCollectionController.prototype.resolve = function() {
  var onResolve = this.scope_['onResolve'];
  if (onResolve && this.responseData) {
      var flow_id = this.responseData['session_id'];
      onResolve({flowId: flow_id});
  }
};

NewArtifactCollectionController.prototype.reject = function() {
  var onReject = this.scope_['onReject'];
  if (onReject && this.responseError) {
      onReject(this.responseError);
  }
};

NewArtifactCollectionController.prototype.checkTools = function() {
    var self = this;
    var tools = Object.keys(self.tools);

    // If no tools left just make the final request.
    if (tools.length == 0) {
        self.startClientFlow();
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

NewArtifactCollectionController.prototype.startClientFlow = function() {
    var self = this;
    var clientId = this.scope_['clientId'];
    var env = [];
    for (var k in self.params) {
        if (self.params.hasOwnProperty(k)) {
            env.push({key: k, value: self.params[k]});
        }
    }

    this.artifactCollectorRequest = {
        client_id: clientId,
        artifacts: this.names,
        parameters: {
            env: env
        },
        ops_per_second: this.ops_per_second,
        timeout: this.timeout,
        max_rows: this.max_rows,
        max_upload_bytes: this.max_bytes * 1024 * 1024,
    };

    this.grrApiService_.post(
        'v1/CollectArtifact',
        this.artifactCollectorRequest).then(function success(response) {
            this.responseData = response['data'];
            this.resolve();
        }.bind(this), function failure(response) {
            this.responseError = response['data']['error'] || 'Unknown error';
            this.reject();
        }.bind(this));
    this.requestSent = true;
};


/**
 * NewArtifactCollectionDirective definition.

 * @return {angular.Directive} Directive definition object.
 */
exports.NewArtifactCollectionDirective = function() {
  return {
    scope: {
        clientId: '=?',
        flowId: "=",
        onResolve: '&',
        onReject: '&'
    },
    restrict: 'E',
    templateUrl: window.base_path+'/static/angular-components/flow/new_artifact_collection.html',
    controller: NewArtifactCollectionController,
    controllerAs: 'controller'
  };
};


/**
 * Directive's name in Angular.
 *
 * @const
 * @export
 */
exports.NewArtifactCollectionDirective.directive_name = 'grrNewArtifactCollection';
