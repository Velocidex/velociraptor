'use strict';

goog.module('grrUi.flow.flowFormDirective');
goog.module.declareLegacyNamespace();

const {valueHasErrors} = goog.require('grrUi.forms.utils');
const {debug} = goog.require('grrUi.core.utils');

/**
 * Controller for FlowFormDirective.
 *
 * @param {!angular.Scope} $scope
 * @param {!grrUi.core.reflectionService.ReflectionService} grrReflectionService
 * @constructor
 * @ngInject
 */
const FlowFormController = function(
    $scope, grrReflectionService) {

  /** @private {!angular.Scope} */
  this.scope_ = $scope;

  /** @private {!grrUi.core.reflectionService.ReflectionService} */
  this.grrReflectionService_ = grrReflectionService;

  debug("FlowFormDirective", this.scope_.flowArgs);

  this.scope_.$watch('flowArgs', this.onArgsDeepChange_.bind(this));
};


/**
 * @private
 */
FlowFormController.prototype.onArgsDeepChange_ = function(value) {
  this.scope_.flowArgs = value;
  debug("FlowFormDirective onArgsDeepChange_", this.scope_.flowArgs);
};

/**
 * Displays a form to edit the flow.
 *
 * @return {angular.Directive} Directive definition object.
 */
exports.FlowFormDirective = function() {
  return {
    scope: {
      flowArgs: '=',
      flowRunnerArgs: '=',
      hasErrors: '=?'
    },
    restrict: 'E',
    templateUrl: '/static/angular-components/flow/flow-form.html',
    controller: FlowFormController,
    controllerAs: 'controller'
  };
};


/**
 * Directive's name in Angular.
 *
 * @const
 * @export
 */
exports.FlowFormDirective.directive_name = 'grrFlowForm';
