'use strict';

goog.module('grrUi.artifact.serverArtifactsDirective');

const serverArtifactsController = function(
  $scope, grrRoutingService) {
  /** @private {!angular.Scope} */
  this.scope_ = $scope;

  /** @private {!grrUi.routing.routingService.RoutingService} */
  this.grrRoutingService_ = grrRoutingService;

  /** @type {string} */
  this.selectedFlowId;

  /** @type {string} */
  this.tab;

  this.scope_.$watchGroup(
      ['controller.selectedFlowId', 'controller.tab'],
      this.onSelectionOrTabChange_.bind(this));

  this.grrRoutingService_.uiOnParamsChanged(
    this.scope_, ['flowId', 'tab'],
      this.onRoutingParamsChange_.bind(this));
};


/**
 * Handles changes to the client id state param.
 *
 * @param {Array} unused_newValues The new values for the watched params.
 * @param {Object=} opt_stateParams A dictionary of all state params and their values.
 * @private
 */
serverArtifactsController.prototype.onRoutingParamsChange_ = function(
    unused_newValues, opt_stateParams) {
    this.selectedFlowId = opt_stateParams['flowId'];
    this.tab = opt_stateParams['tab'];
};


serverArtifactsController.prototype.newArtifactCollection = function() {
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
    template: '<grr-new-artifact-collection client-id="\'server\'" '+
      'on-resolve="resolve()" on-reject="reject()" />',
    scope: modalScope,
    windowClass: 'wide-modal high-modal',
    size: 'lg'
  });
};


serverArtifactsController.prototype.onSelectionOrTabChange_ = function() {
  if (angular.isDefined(this.selectedFlowId)) {
      this.grrRoutingService_.go(
          'server_artifacts',
          {flowId: this.selectedFlowId, tab: this.tab});
  }
};

exports.ServerArtifactsDirective = function() {
  return {
    scope: {
      "artifact": '=',
    },
    restrict: 'E',
    templateUrl: window.base_path+'/static/angular-components/artifact/server-artifacts.html',
    controller: serverArtifactsController,
    controllerAs: 'controller'
  };
};


exports.ServerArtifactsDirective.directive_name = 'grrServerArtifacts';
