'use strict';

goog.module('grrUi.hunt.newHuntWizard.formDirective');
goog.module.declareLegacyNamespace();

const {ApiService, stripTypeInfo} = goog.require('grrUi.core.apiService');
const {ReflectionService} = goog.require('grrUi.core.reflectionService');
const {debug} = goog.require('grrUi.core.utils');

/**
 * Controller for FormDirective.
 *
 * @param {!angular.Scope} $scope
 * @param {!ReflectionService} grrReflectionService
 * @param {!ApiService} grrApiService
 * @constructor
 * @ngInject
 */
const FormController = function($scope, grrReflectionService, grrApiService) {
    /** @private {!angular.Scope} */
    this.scope_ = $scope;

    /** @private {!ReflectionService} */
    this.grrReflectionService_ = grrReflectionService;

    /** @private {!ApiService} */
    this.grrApiService_ = grrApiService;

    if (angular.isUndefined(this.scope_['createHuntArgs'])) {
        this.scope_['createHuntArgs'] = {
            start_request: {
                "flow_name": "ArtifactCollector",
                args: {
                    "@type": "type.googleapis.com/proto.ArtifactCollectorArgs",
                },
            },
            condition: {},
        };
    }
};


/**
 * Sends hunt creation request to the server.
 *
 * @export
 */
FormController.prototype.sendRequest = function() {
  this.grrApiService_.post(
      'v1/CreateHunt',
      this.scope_['createHuntArgs'])
  .then(function resolve(response) {
    this.serverResponse = response;
  }.bind(this), function reject(response) {
    this.serverResponse = response;
    this.serverResponse['error'] = true;
  }.bind(this));
};


/**
 * Called when the wizard resolves. Instead of directly calling the
 * scope callback, this controller method adds additional information (hunt id)
 * to the callback.
 *
 * @export
 */
FormController.prototype.resolve = function() {
  var onResolve = this.scope_['onResolve'];
  if (onResolve && this.serverResponse) {
    var huntId = this.serverResponse['data']['flow_id'];
    onResolve({huntId: huntId});
  }
};


/**
 * Directive for showing wizard-like forms with multiple named steps/pages.
 *
 * @return {!angular.Directive} Directive definition object.
 * @ngInject
 * @export
 */
exports.FormDirective = function() {
  return {
    scope: {
      createHuntArgs: '=?',
      onResolve: '&',
      onReject: '&'
    },
    restrict: 'E',
    templateUrl: '/static/angular-components/hunt/new-hunt-wizard/form.html',
    controller: FormController,
    controllerAs: 'controller'
  };
};


/**
 * Directive's name in Angular.
 *
 * @const
 * @export
 */
exports.FormDirective.directive_name = 'grrNewHuntWizardForm';
