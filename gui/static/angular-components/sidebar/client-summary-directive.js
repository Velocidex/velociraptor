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
const ClientSummaryController = function($scope, $interval,
                                         grrApiService,
                                         grrTimeService,
                                         grrRoutingService) {
    /** @private {!angular.Scope} */
    this.scope_ = $scope;

    /** @private {!grrUi.core.apiService.ApiService} */
    this.grrApiService_ = grrApiService;

    /** @private {!grrUi.core.timeService.TimeService} */
    this.grrTimeService_ = grrTimeService;

    /** @private {!RoutingService} */
    this.grrRoutingService_ = grrRoutingService;

    /** @type {object} */
    this.clientInfo = null;

    // How long did we see this client.
    this.seen_ago;

    this.clientId;

    this.scope_.$watch('clientId',
                       this.onClientSelectionChange_.bind(this));

    // Subscribe to legacy grr events to be notified on client change.
    this.grrRoutingService_.uiOnParamsChanged(
        this.scope_, 'clientId',
        this.onClientSelectionChange_.bind(this), true);

    this.stop = $interval(this.onInterval_.bind(this), 1000);

    // Refresh client information every 10 seconds to ensure we pick
    // up clients coming online.
    this.clientDetailsRefresher = $interval(function() {
        if (angular.isDefined(this.clientId)) {
            var url = 'v1/GetClient/' + this.clientId;
            this.grrApiService_.get(url).then(this.onClientDetailsFetched_.bind(this));
        }
    }.bind(this), 10000);

    this.scope_.$on('$destroy', function() {
        $interval.cancel(this.stop);
        $interval.cancel(this.clientDetailsRefresher);
    });

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

    if (this.clientId === clientId) {
        return;
    }
    this.clientId = clientId;

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

ClientSummaryController.prototype.onInterval_ = function() {
    if (!angular.isDefined(this.clientInfo)) {
        return;
    }

    var last_seen_at = this.clientInfo['last_seen_at'] || 0;
    var currentTimeMs = this.grrTimeService_.getCurrentTimeMs();
    var inputTimeMs = last_seen_at / 1000;

    if (inputTimeMs < 1e-6) {
        return '<invalid time value>';
    }

    var differenceSec = Math.abs(
        Math.round((currentTimeMs - inputTimeMs) / 1000));
    this.last_seen_sec = differenceSec;

    var measureUnit;
    var measureValue;
    if (differenceSec < 15) {
        this.seen_ago = "connected";
        return;
    } else if (differenceSec < 60) {
        measureUnit = 'seconds';
        measureValue = differenceSec;
    } else if (differenceSec < 60 * 60) {
        measureUnit = 'minutes';
        measureValue = Math.floor(differenceSec / 60);
    } else if (differenceSec < 60 * 60 * 24) {
        measureUnit = 'hours';
        measureValue = Math.floor(differenceSec / (60 * 60));
    } else {
        measureUnit = 'days';
        measureValue = Math.floor(differenceSec / (60 * 60 * 24));
    }

    if (currentTimeMs >= inputTimeMs) {
        this.seen_ago = measureValue + ' ' + measureUnit + ' ago';
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
