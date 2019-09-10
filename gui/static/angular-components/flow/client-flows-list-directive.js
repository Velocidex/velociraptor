'use strict';

goog.module('grrUi.flow.clientFlowsListDirective');
goog.module.declareLegacyNamespace();



/**
 * Controller for FlowsListDirective.
 *
 * @constructor
 * @param {!angular.Scope} $scope
 * @param {!angular.$timeout} $timeout
 * @param {!angularUi.$uibModal} $uibModal
 * @param {!grrUi.core.apiService.ApiService} grrApiService
 * @param {!grrUi.routing.routingService.RoutingService} grrRoutingService
 * @ngInject
 */
const ClientFlowsListController = function(
    $scope, $timeout, $uibModal, grrApiService, grrRoutingService) {
  /** @private {!angular.Scope} */
  this.scope_ = $scope;

  /** @private {!angular.$timeout} */
  this.timeout_ = $timeout;

  /** @private {!angularUi.$uibModal} */
  this.uibModal_ = $uibModal;

  /** @private {!grrUi.core.apiService.ApiService} */
  this.grrApiService_ = grrApiService;

  /** @private {!grrUi.routing.routingService.RoutingService} */
  this.grrRoutingService_ = grrRoutingService;

  /** @type {?string} */
    this.flowsUrl;

  /**
   * This variable is bound to grr-flows-list's trigger-update attribute
   * and therefore is set by that directive to a function that triggers
   * list update.
   * @export {function()}
   */
  this.triggerUpdate;

  this.scope_.$watch('clientId', this.onClientIdChange_.bind(this));
};

/**
 * Handles changes of clientId binding.
 *
 * @param {?string} newValue New binding value.
 * @private
 */
ClientFlowsListController.prototype.onClientIdChange_ = function(newValue) {
  if (angular.isString(newValue)) {
    var components = newValue.split('/');
    var basename = components[components.length - 1];
    this.flowsUrl = '/v1/GetClientFlows/' + basename;
  } else {
    this.flowsUrl = null;
  }
};

/**
 * Handles clicks on 'Cancel Flow' button.
 *
 * @export
 */
ClientFlowsListController.prototype.cancelButtonClicked = function() {
    this.grrApiService_.post('/v1/CancelFlow', {
        client_id: this.scope_['clientId'],
        flow_id: this.scope_['selectedFlowId'],
    }).then(function() {
        this.triggerUpdate();

        // This will force all the directives that depend on selectedFlowId
        // binding to refresh.
        var flowId = this.scope_['selectedFlowId'];
        this.scope_['selectedFlowId'] = undefined;
        this.timeout_(function() {
            this.scope_['selectedFlowId'] = flowId;
        }.bind(this), 0);
    }.bind(this));
};

/**
 * Handles clicks on 'Delete Flow' button.
 *
 * @export
 */
ClientFlowsListController.prototype.deleteButtonClicked = function() {
    this.grrApiService_.post('/v1/ArchiveFlow', {
        client_id: this.scope_['clientId'],
        flow_id: this.scope_['selectedFlowId'],
    }).then(function() {
        this.triggerUpdate();
        this.scope_['selectedFlowId'] = undefined;
    }.bind(this));
};

/**
 * Shows a 'New Hunt' dialog prefilled with the data of the currently selected
 * flow.
 *
 * @export
 */
ClientFlowsListController.prototype.createHuntFromFlow = function() {
  var huntId;

  var modalScope = this.scope_.$new();
  modalScope['clientId'] = this.scope_['clientId'];
  modalScope['flowId'] = this.scope_['selectedFlowId'];
  modalScope['resolve'] = function(newHuntId) {
    huntId = newHuntId;
    modalInstance.close();
  }.bind(this);
  modalScope['reject'] = function() {
    modalInstance.dismiss();
  }.bind(this);

  this.scope_.$on('$destroy', function() {
    modalScope.$destroy();
  });

  var modalInstance = this.uibModal_.open({
    template: '<grr-new-hunt-wizard-create-from-flow-form on-resolve="resolve(huntId)" ' +
        'on-reject="reject()" flow-id="flowId" client-id="clientId" />',
    scope: modalScope,
    windowClass: 'wide-modal high-modal',
    size: 'lg'
  });
  modalInstance.result.then(function resolve() {
    this.grrRoutingService_.go('hunts', {huntId: huntId});
  }.bind(this));
};


/**
 * Shows a 'New Hunt' dialog prefilled with the data of the currently selected
 * hunt.
 *
 * @export
 */
ClientFlowsListController.prototype.copyFlow = function() {
  var newFlowId;

  var modalScope = this.scope_.$new();
  modalScope['clientId'] = this.scope_['clientId'];
  modalScope['flowId'] = this.scope_['selectedFlowId'];
  modalScope['resolve'] = function(newFlowObj) {
    newFlowId = newFlowObj['flow_id'];
    modalInstance.close();
  }.bind(this);

  this.scope_.$on('$destroy', function() {
    modalScope.$destroy();
  });

  var modalInstance = this.uibModal_.open({
    template: '<grr-copy-flow-form on-resolve="resolve(flow)" ' +
        'flow-id="flowId" client-id="clientId" />',
    scope: modalScope,
    windowClass: 'wide-modal high-modal',
    size: 'lg'
  });
  modalInstance.result.then(function resolve() {
    // newFlowId will remain unset if an error happened on the server and
    // 'resolve' callback was never called.
    if (angular.isDefined(newFlowId)) {
      this.grrRoutingService_.go('client.flows', {flowId: newFlowId});
      this.triggerUpdate();
    }
  }.bind(this));
};

/**
 * Shows new hunt wizard.
 *
 * @export
 */
ClientFlowsListController.prototype.newArtifactCollection = function() {
    var modalScope = this.scope_.$new();
    var self = this;

  modalScope.resolve = function() {
      modalInstance.close();
      self.triggerUpdate();
  };
  modalScope.reject = function() {
    modalInstance.dismiss();
  };
  this.scope_.$on('$destroy', function() {
    modalScope.$destroy();
  });

  var modalInstance = this.uibModal_.open({
    template: '<grr-new-artifact-collection client-id="clientId" '+
          'on-resolve="resolve()" on-reject="reject()" />',
    scope: modalScope,
    windowClass: 'wide-modal high-modal',
    size: 'lg'
  });
};


/**
 * FlowsListDirective definition.

 * @return {angular.Directive} Directive definition object.
 */
exports.ClientFlowsListDirective = function() {
  return {
    scope: {
      clientId: '=',
      selectedFlowId: '=?',
      selectedFlowState: '=?',
      triggerUpdate: '=?',
    },
    restrict: 'E',
    templateUrl: '/static/angular-components/flow/client-flows-list.html',
    controller: ClientFlowsListController,
    controllerAs: 'controller'
  };
};


/**
 * Directive's name in Angular.
 *
 * @const
 * @export
 */
exports.ClientFlowsListDirective.directive_name = 'grrClientFlowsList';
