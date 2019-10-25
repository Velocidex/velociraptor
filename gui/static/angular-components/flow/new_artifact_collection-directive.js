'use strict';

goog.module('grrUi.flow.newArtifactCollectionDirective');

const {ApiService, stripTypeInfo} = goog.require('grrUi.core.apiService');


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

  /** @type {boolean} */
  this.flowFormHasErrors;

  this.params = {};
  this.names = [];
  this.ops_per_second;
  this.timeout = 600;
};


NewArtifactCollectionController.prototype.resolve = function() {
  var onResolve = this.scope_['onResolve'];
  if (onResolve && this.responseData) {
      var flow_id = this.responseData['session_id'];
      onResolve({flowId: flow_id});
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
        request: {
            artifacts: {
                names: this.names
            },
            parameters: {
                env: env
            },
            ops_per_second: this.ops_per_second,
            timeout: this.timeout
        }
    };

    this.grrApiService_.post(
        'v1/CollectArtifact',
        this.artifactCollectorRequest).then(function success(response) {
            this.responseData = response['data'];
        }.bind(this), function failure(response) {
            this.responseError = response['data']['error'] || 'Unknown error';
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
