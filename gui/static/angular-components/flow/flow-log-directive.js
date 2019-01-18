'use strict';

goog.module('grrUi.flow.flowLogDirective');
goog.module.declareLegacyNamespace();

/**
 * Controller for FlowLogDirective.
 *
 * @constructor
 * @param {!angular.Scope} $scope
 * @ngInject
 */
const FlowLogController = function($scope) {
  /** @private {!angular.Scope} */
  this.scope_ = $scope;

    this.scope_.$watch('flowId',
                       this.onFlowIdChange_.bind(this));

    this.scope_.$watch('clientId',
                       this.onClientIdChange_.bind(this));

    this.queryParams = {};
};

/**
 * Handles flowId attribute changes.
 *
 * @private
 */
FlowLogController.prototype.onFlowIdChange_ = function(newValue) {
    this.queryParams.path = '/flows/' + newValue + '/logs';
};

FlowLogController.prototype.onClientIdChange_ = function(newValue) {
        this.queryParams.client_id = newValue;
};


/**
 * Directive for displaying logs of a flow with a given URN.
 *
 * @return {!angular.Directive} Directive definition object.
 * @ngInject
 * @export
 */
exports.FlowLogDirective = function() {
  return {
      scope: {
          flowId: '=',
          clientId: '=',
      },
      restrict: 'E',
      templateUrl: '/static/angular-components/flow/flow-log.html',
      controller: FlowLogController,
      controllerAs: 'controller'
  };
};

/**
 * Directive's name in Angular.
 *
 * @const
 * @export
 */
exports.FlowLogDirective.directive_name = 'grrFlowLog';
