'use strict';

goog.module('grrUi.sidebar.clientSummaryDirective');
goog.module.declareLegacyNamespace();



/**
 * Controller for ClientSummaryDirective.
 *
 * @constructor
 * @param {!angular.Scope} $scope
 * @param {!grrUi.core.apiService.ApiService} grrApiService
 * @param {!grrUi.core.timeService.TimeService} grrTimeService
 * @param {!RoutingService} grrRoutingService
 * @ngInject
 */
const ClientSummaryController =
      function($scope, grrApiService, grrTimeService, grrRoutingService) {

  /** @private {!angular.Scope} */
  this.scope_ = $scope;

  /** @private {!grrUi.core.apiService.ApiService} */
  this.grrApiService_ = grrApiService;

  /** @private {!grrUi.core.timeService.TimeService} */
  this.grrTimeService_ = grrTimeService;

  /** @private {!RoutingService} */
  this.grrRoutingService_ = grrRoutingService;

  /** @type {string} */
  this.approvalReason;

    /** @type {object} */
  this.clientInfo = null;
  this.clientId;

  this.scope_.$watch('clientId', this.onClientSelectionChange_.bind(this));

  // Subscribe to legacy grr events to be notified on client change.
  this.grrRoutingService_.uiOnParamsChanged(this.scope_, 'clientId',
      this.onClientSelectionChange_.bind(this), true);

};

/**
 * Handles selection of a client.
 *
 * @param {string} clientId The id of the selected client.
 * @private
 */
ClientSummaryController.prototype.onClientSelectionChange_ = function(clientId) {
  if (!clientId) {
    return; // Stil display the last client for convenience.
  }
  if (clientId.indexOf('aff4:/') === 0) {
    clientId = clientId.split('/')[1];
  }
  if (this.clientId === clientId) {
    return;
  }

  var url = 'v1/GetClient/' + clientId;
  this.grrApiService_.get(url).then(this.onClientDetailsFetched_.bind(this));
};

/**
 * Called when the client details were fetched.
 *
 * @param {Object} response
 * @private
 */
ClientSummaryController.prototype.onClientDetailsFetched_ = function(response) {
    this.clientInfo = response['data'];

  // Check for the last crash.
  if (this.clientInfo['last_crash_at']){
    var currentTimeMs = this.grrTimeService_.getCurrentTimeMs();
    var crashTime = this.clientInfo['last_crash_at'];
    if (angular.isDefined(crashTime) &&
        (currentTimeMs / 1000 - crashTime / 1000000) < 60 * 60 * 24) {
      this.crashTime = crashTime;
    }
  }
};


/**
 * Directive for displaying a client summary for the navigation.
 *
 * @return {!angular.Directive} Directive definition object.
 * @ngInject
 * @export
 */
exports.ClientSummaryDirective = function() {
  return {
    scope: {
      client_id: '='
    },
    restrict: 'E',
    templateUrl: '/static/angular-components/sidebar/client-summary.html',
    controller: ClientSummaryController,
    controllerAs: 'controller'
  };
};


/**
 * Directive's name in Angular.
 *
 * @const
 * @export
 */
exports.ClientSummaryDirective.directive_name = 'grrClientSummary';
