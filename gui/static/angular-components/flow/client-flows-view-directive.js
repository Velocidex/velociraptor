'use strict';

goog.module('grrUi.flow.clientFlowsViewDirective');
goog.module.declareLegacyNamespace();



/**
 * Controller for ClientFlowsViewDirective.
 *
 * @constructor
 * @param {!angular.Scope} $scope
 * @param {!grrUi.routing.routingService.RoutingService} grrRoutingService
 * @ngInject
 */
const ClientFlowsViewController = function(
    $scope, grrRoutingService) {
  /** @private {!angular.Scope} */
  this.scope_ = $scope;

    /** @private {!grrUi.routing.routingService.RoutingService} */
    this.grrRoutingService_ = grrRoutingService;

    /** @type {string} */
    this.clientId = this.scope_["clientId"];

    /** @type {string} */
    this.selectedFlowId;

    /** @type {string} */
    this.tab;

    this.grrRoutingService_.uiOnParamsChanged(
        this.scope_, ['clientId', 'flowId', 'tab'],
        this.onRoutingParamsChange_.bind(this));

    this.scope_.$watchGroup(
        ['controller.tab', 'controller.clientId', 'controller.selectedFlowId'],
        this.onSelectionChange_.bind(this));
};


ClientFlowsViewController.prototype.onSelectionChange_ = function() {
    if (angular.isString(this.selectedFlowId)) {
        if (!angular.isString(this.clientId) || this.clientId == "server") {
            this.grrRoutingService_.go("server_artifacts", {
                flowId: this.selectedFlowId,
                tab: this.tab
            });
        } else {
            this.grrRoutingService_.go("client.flows", {
                flowId: this.selectedFlowId,
                clientId: this.clientId,
                tab: this.tab
            });
        };
  }
};

/**
 * Handles changes to the client id state param.
 *
 * @param {Array} unused_newValues The new values for the watched params.
 * @param {Object=} opt_stateParams A dictionary of all state params and their values.
 * @private
 */
ClientFlowsViewController.prototype.onRoutingParamsChange_ = function(
    unused_newValues, opt_stateParams) {
    if (opt_stateParams["clientId"]) {
        this.clientId = opt_stateParams["clientId"];
    }

    if (opt_stateParams['flowId']) {
        this.selectedFlowId = opt_stateParams['flowId'];
        this.tab = opt_stateParams['tab'];
    }
};

/**
 * FlowsViewDirective definition.

 * @return {angular.Directive} Directive definition object.
 */
exports.ClientFlowsViewDirective = function() {
  return {
    scope: {
      clientId: '=',
    },
    restrict: 'E',
    templateUrl: window.base_path+'/static/angular-components/flow/client-flows-view.html',
    controller: ClientFlowsViewController,
    controllerAs: 'controller'
  };
};


/**
 * Directive's name in Angular.
 *
 * @const
 * @export
 */
exports.ClientFlowsViewDirective.directive_name = 'grrClientFlowsView';
