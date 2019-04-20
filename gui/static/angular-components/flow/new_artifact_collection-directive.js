'use strict';

goog.module('grrUi.flow.newArtifactCollectionDirective');
goog.module.declareLegacyNamespace();

const {ApiService, stripTypeInfo} = goog.require('grrUi.core.apiService');
const {ReflectionService} = goog.require('grrUi.core.reflectionService');


/**
 * Controller for NewArtifactCollectionController.
 *
 * @constructor
 * @param {!angular.Scope} $scope
 * @param {!ApiService} grrApiService
 * @param {!ReflectionService} grrReflectionService
 * @ngInject
 */
const NewArtifactCollectionController = function(
    $scope, grrApiService, grrReflectionService) {
  /** @private {!angular.Scope} */
  this.scope_ = $scope;

  /** @type {string} */
  this.scope_.clientId;

  /** @private {!ApiService} */
  this.grrApiService_ = grrApiService;

  /** @private {!ReflectionService} */
  this.grrReflectionService_ = grrReflectionService;

  /** @type {Object} */
    this.flowArguments = {
        "@type": "type.googleapis.com/proto.ArtifactCollectorArgs"
    };

  /** @type {Object} */
    this.flowRunnerArguments = {
        "flow_name": "ArtifactCollector",
    };

  /** @type {boolean} */
  this.requestSent = false;

  /** @type {?string} */
  this.responseError;

  /** @type {?string} */
  this.responseData;

  /** @type {boolean} */
  this.flowFormHasErrors;
};


NewArtifactCollectionController.prototype.resolve = function() {
  var onResolve = this.scope_['onResolve'];
  if (onResolve && this.responseData) {
      var flow_id = this.responseData['flow_id'];
      onResolve({flowId: flow_id});
  }
};


/**
 * Sends API request to start a client flow.
 *
 * @export
 */
NewArtifactCollectionController.prototype.startClientFlow = function() {
  var clientIdComponents = this.scope_['clientId'].split('/');
  var clientId;
  if (clientIdComponents[0] == 'aff4:') {
    clientId = clientIdComponents[1];
  } else {
    clientId = clientIdComponents[0];
  }

  this.flowRunnerArguments.client_id = clientId;
  this.flowRunnerArguments.args = this.flowArguments;
  this.grrApiService_.post(
    'v1/LaunchFlow',
    this.flowRunnerArguments).then(function success(response) {
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
