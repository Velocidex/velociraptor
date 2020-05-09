'use strict';

goog.module('grrUi.flow.flowRequestsDirective');
goog.module.declareLegacyNamespace();



/**
 * Controller for FlowRequestsDirective.
 *
 * @constructor
 * @param {!angular.Scope} $scope
 * @ngInject
 */
const FlowRequestsController = function($scope) {
  /** @private {!angular.Scope} */
  this.scope_ = $scope;

    /** @type {string} */
    this.requestsUrl;

    /** @type {object} */
    this.requestsParams;

  this.scope_.$watch('flowId', this.onFlowIdPathChange_.bind(this));
};



/**
 * Handles directive's arguments changes.
 *
 * @param {Array<string>} newValues
 * @private
 */
FlowRequestsController.prototype.onFlowIdPathChange_ = function() {
    this.requestsUrl = "v1/GetFlowRequests";
    this.requestsParams = {
        flow_id: this.scope_['flowId'],
        client_id: this.scope_['clientId'],
    };
};


/**
 * Directive for displaying requests of a flow with a given URN.
 *
 * @return {angular.Directive} Directive definition object.
 * @ngInject
 * @export
 */
exports.FlowRequestsDirective = function() {
  return {
    scope: {
      flowId: '=',
      clientId: '=',
    },
    restrict: 'E',
    templateUrl: '/static/angular-components/flow/flow-requests.html',
    controller: FlowRequestsController,
    controllerAs: 'controller'
  };
};

/**
 * Directive's name in Angular.
 *
 * @const
 * @export
 */
exports.FlowRequestsDirective.directive_name = 'grrFlowRequests';
