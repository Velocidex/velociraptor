'use strict';

goog.module('grrUi.client.hostInfoDirective');
goog.module.declareLegacyNamespace();



var OPERATION_POLL_INTERVAL_MS = 1000;

/**
 * Controller for HostInfoDirective.
 *
 * @constructor
 * @param {!angular.Scope} $scope
 * @param {!angular.$interval} $interval
 * @param {!grrUi.core.apiService.ApiService} grrApiService
 * @param {!grrUi.routing.routingService.RoutingService} grrRoutingService
 * @param {!grrUi.core.dialogService.DialogService} grrDialogService
 * @ngInject
 */
const HostInfoController = function(
    $scope, $interval, grrApiService, grrRoutingService, $uibModal,
    grrDialogService) {

    /** @private {!angular.Scope} */
    this.scope_ = $scope;

    this.uibModal_ = $uibModal;

    /** @private {!angular.$interval} */
    this.interval_ = $interval;

    /** @private {!grrUi.core.apiService.ApiService} */
    this.grrApiService_ = grrApiService;

    /** @private {!grrUi.routing.routingService.RoutingService} */
    this.grrRoutingService_ = grrRoutingService;

    /** @private {!grrUi.core.dialogService.DialogService} */
    this.grrDialogService_ = grrDialogService;

    /** @type {string} */
    this.clientId;

    /** @export {Object} */
    this.client;

    /** @type {boolean} */
    this.hasClientAccess;

    /** @type {number} */
    this.fetchDetailsRequestId = 0;

    /** @type {?string} */
    this.interrogateOperationId;

    /** @private {!angular.$q.Promise} */
    this.interrogateOperationInterval_;

    // clientId may be inferred from the URL or passed explicitly as
    // a parameter.
    this.grrRoutingService_.uiOnParamsChanged(this.scope_, 'clientId',
                                              this.onClientIdChange_.bind(this));
    this.scope_.$watch('clientId', this.onClientIdChange_.bind(this));

    // TODO(user): use grrApiService.poll for polling.
    this.scope_.$on('$destroy',
                    this.stopMonitorInterrogateOperation_.bind(this));

    this.report_params = {};
};



/**
 * Handles changes to the client id.
 *
 * @param {string} clientId The new value for the client id state param.
 * @private
 */
HostInfoController.prototype.onClientIdChange_ = function(clientId) {
  if (angular.isDefined(clientId)) {
    this.clientId = clientId;
    this.fetchClientDetails_();
  }
};

/**
 * Fetches the client details.
 *
 * @private
 */
HostInfoController.prototype.fetchClientDetails_ = function() {
  var url = '/v1/GetClient/' + this.clientId;
  var params = {details: true};

  this.fetchDetailsRequestId += 1;
  var requestId = this.fetchDetailsRequestId;
    this.grrApiService_.get(url, params).then(function success(response) {
    // Make sure that the request that we got corresponds to the
    // arguments we used while sending it. This is needed for cases
    // when bindings change so fast that we send multiple concurrent
    // requests.
    if (this.fetchDetailsRequestId != requestId) {
      return;
    }

    this.client = response.data;
    this.report_params = {
      artifact: 'Custom.Generic.Client.Info',
      client_id: this.client.client_id,
      flowId: this.client.last_interrogate_flow_id,
      type: 'CLIENT'
    };

    }.bind(this));
};

/**
 * Starts the interrogation of the client.
 *
 * @export
 */
HostInfoController.prototype.interrogate = function() {
    var url = '/v1/CollectArtifact';
    var params ={
        'client_id': this.clientId,
        'artifacts': ['Generic.Client.Info'],
    };

    this.grrApiService_.post(url, params).then(
        function success(response) {
            this.interrogateOperationId = response['data']['flow_id'];
            this.monitorInterrogateOperation_();
        }.bind(this),
        function failure(response) {
            this.stopMonitorInterrogateOperation_();
        }.bind(this));
};

HostInfoController.prototype.buildCollector = function() {
  var modalScope = this.scope_.$new();
  modalScope.resolve = function() {
    modalInstance.close();
  };
  modalScope.reject = function() {
    modalInstance.dismiss();
  };
  this.scope_.$on('$destroy', function() {
    modalScope.$destroy();
  });

  var modalInstance = this.uibModal_.open({
    template: '<grr-build-collector on-resolve="resolve()" ' +
        'on-reject="reject()" />',
    scope: modalScope,
    windowClass: 'wide-modal high-modal',
    size: 'lg'
  });

  modalInstance.result.then(function resolve() {
    this.triggerUpdate();
  }.bind(this));
};


/**
 * Polls the interrogate operation state.
 *
 * @private
 */
HostInfoController.prototype.monitorInterrogateOperation_ = function() {
  this.interrogateOperationInterval_ = this.interval_(
      this.pollInterrogateOperationState_.bind(this),
      OPERATION_POLL_INTERVAL_MS);
};

/**
 * Polls the state of the interrogate operation.
 *
 * @private
 */
HostInfoController.prototype.pollInterrogateOperationState_ = function() {
    var url = 'v1/GetFlowDetails/';
    var param = {flow_id: this.interrogateOperationId,
                 client_id: this.clientId};

    this.grrApiService_.get(url, param).then(
        function success(response) {
            if (response['data']['context']['state'] != 'RUNNING') {
                this.stopMonitorInterrogateOperation_();

                this.fetchClientDetails_();
            }
        }.bind(this),
        function failure(response) {
            this.stopMonitorInterrogateOperation_();
        }.bind(this));
};

/**
 * Stop polling for the state of the interrogate operation.
 *
 * @private
 */
HostInfoController.prototype.stopMonitorInterrogateOperation_ = function() {
  this.interrogateOperationId = null;
  this.interval_.cancel(this.interrogateOperationInterval_);
};

/**
 * HostInfoDirective definition.
 *
 * @return {angular.Directive} Directive definition object.
 */
exports.HostInfoDirective = function() {
  return {
    scope: {
      'clientId': '='
    },
    restrict: 'E',
    templateUrl: '/static/angular-components/client/host-info.html',
    controller: HostInfoController,
    controllerAs: 'controller'
  };
};


/**
 * Name of the directive in Angular.
 *
 * @const
 * @export
 */
exports.HostInfoDirective.directive_name = 'grrHostInfo';
