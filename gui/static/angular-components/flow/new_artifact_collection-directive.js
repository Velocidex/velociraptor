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

    // The names of the artifacts to collect.
    this.names = [];
    this.ops_per_second = 0;
    this.timeout = 600;

    this.flow_details = {};

    this.scope_.$watchGroup(['clientId', 'flowId'],
                            this.onClientIdChange_.bind(this));
};


NewArtifactCollectionController.prototype.onClientIdChange_ = function() {
    var url = 'v1/GetFlowDetails';
    var param = {flow_id: this.scope_["flowId"],
                 client_id: this.scope_["clientId"]};

    this.grrApiService_.get(url, param).then(
        function success(response) {
            this.flow_details = response.data["context"];
            this.names = this.flow_details.request.artifacts;
            this.params = {};

            var env = this.flow_details.request.parameters.env;
            for (var i=0; i<env.length;i++) {
                var key = env[i]["key"];
                var value = env[i]["value"];
                if (angular.isString(key)) {
                    this.params[key] = value;
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


/**
 * Sends API request to start a client flow.
 *
 * @export
 */
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
        timeout: this.timeout
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
    templateUrl: '/static/angular-components/flow/new_artifact_collection.html',
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
