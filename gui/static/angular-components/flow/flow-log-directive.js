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

  this.scope_.$watchGroup(['flowId', 'clientId'],
                          this.onChange_.bind(this));

  this.queryParams = {};
  this.onChange_();
};

/**
 * Handles flowId attribute changes.
 *
 * @private
 */
FlowLogController.prototype.onChange_ = function() {
  this.queryParams.path = '/flows/' + this.scope_['flowId'] + '/logs';;
  this.queryParams.client_id = this.scope_["clientId"];
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
