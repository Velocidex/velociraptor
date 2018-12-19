'use strict';

goog.module('grrUi.flow.flowOverviewDirective');
goog.module.declareLegacyNamespace();


/**
 * Controller for FlowOverviewDirective.
 *
 * @constructor
 * @param {!angular.Scope} $scope
 * @param {!grrUi.core.apiService.ApiService} grrApiService
 * @ngInject
 */
const FlowOverviewController = function(
    $scope, grrApiService) {
  /** @private {!angular.Scope} */
  this.scope_ = $scope;
};


/**
 * Directive for displaying log records of a flow with a given URN.
 *
 * @return {angular.Directive} Directive definition object.
 * @ngInject
 * @export
 */
exports.FlowOverviewDirective = function() {
  return {
      scope: {
          flow: '=',
          flowId: '=',
          apiBasePath: '='
      },
      restrict: 'E',
      templateUrl: '/static/angular-components/flow/flow-overview.html',
      controller: FlowOverviewController,
      controllerAs: 'controller'
  };
};


/**
 * Directive's name in Angular.
 *
 * @const
 * @export
 */
exports.FlowOverviewDirective.directive_name = 'grrFlowOverview';
