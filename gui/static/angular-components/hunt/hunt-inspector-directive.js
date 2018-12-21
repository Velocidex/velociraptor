'use strict';

goog.module('grrUi.hunt.huntInspectorDirective');
goog.module.declareLegacyNamespace();

/** @type {number} */
let AUTO_REFRESH_INTERVAL_MS = 15 * 1000;

/**
 * Sets the delay between automatic refreshes.
 *
 * @param {number} millis Interval value in milliseconds.
 * @export
 */
exports.setAutoRefreshInterval = function(millis) {
  AUTO_REFRESH_INTERVAL_MS = millis;
};


/**
 * Controller for HuntInspectorDirective.
 *
 * @constructor
 * @param {!angular.Scope} $scope
 * @ngInject
 */
const HuntInspectorController = function(
    $scope, grrApiService, grrRoutingService) {
  /** @private {!angular.Scope} */
  this.scope_ = $scope;

  /** @type {string} */
  this.shownHuntId;

  /** @type {string} */
  this.activeTab = '';

  /** type {Object<string, boolean>} */
  this.tabsShown = {};

    this.scope_.$watchGroup(['huntId', 'activeTab'],
                            this.onDirectiveArgumentsChange_.bind(this));

    this.scope_.$watch('controller.activeTab', this.onTabChange_.bind(this));

  /** @private {!grrUi.core.apiService.ApiService} */
  this.grrApiService_ = grrApiService;

  /** @private {!grrUi.routing.routingService.RoutingService} */
  this.grrRoutingService_ = grrRoutingService;

  /** @export {string} */
  this.huntId;

  /** @export {Object} */
  this.hunt;

  /** @private {!angular.$q.Promise|undefined} */
  this.pollPromise_;

  this.scope_.$on('$destroy', function() {
    this.grrApiService_.cancelPoll(this.pollPromise_);
  }.bind(this));

  this.scope_.$watch('huntId', this.startPolling_.bind(this));
};

/**
 * Fetches hunt data;
 *
 * @private
 */
HuntInspectorController.prototype.startPolling_ = function() {
  this.grrApiService_.cancelPoll(this.pollPromise_);
  this.pollPromise_ = undefined;

  if (angular.isDefined(this.scope_['huntId'])) {
      // FIXME: Remove the aff4 path from this.
      var huntId = this.scope_['huntId'];
      var components = huntId.split("/");
      this.huntId = components[components.length-1];

    this.pollPromise_ = this.grrApiService_.poll(
      'v1/GetHunt',
      AUTO_REFRESH_INTERVAL_MS,
      {hunt_id: this.huntId});
    this.pollPromise_.then(
        undefined,
        undefined,
        function notify(response) {
            this.hunt = response['data'];
        }.bind(this));
  }
};

/**
 * Handles huntId and activeTab scope attribute changes.
 *
 * @private
 */
HuntInspectorController.prototype.onDirectiveArgumentsChange_ = function() {
    if (angular.isString(this.scope_['activeTab'])) {
        this.activeTab = this.scope_['activeTab'];
    }
};

/**
 * Handles huntId scope attribute changes.
 *
 * @param {string} newValue
 * @param {string} oldValue
 * @private
 */
HuntInspectorController.prototype.onTabChange_ = function(newValue, oldValue) {
    if (newValue !== oldValue) {
        this.scope_['activeTab'] = newValue;
    }
    this.tabsShown[newValue] = true;
};


/**
 * Downloades the file.
 *
 * @export
 */
HuntInspectorController.prototype.downloadFile = function() {
    var url = 'v1/DownloadHuntResults';
    var params = {hunt_id: this.scope_["huntId"]};
    this.grrApiService_.downloadFile(url, params).then(
        function success() {}.bind(this),
        function failure(response) {
            if (angular.isUndefined(response.status)) {
                this.rootScope_.$broadcast(
                    ERROR_EVENT_NAME, {
                        message: 'Couldn\'t download file.'
                    });
            }
        }.bind(this)
    );
};


/**
 * HuntInspectorDirective definition.

 * @return {angular.Directive} Directive definition object.
 */
exports.HuntInspectorDirective = function() {
  return {
    scope: {
      huntId: '=',
      activeTab: '=?'
    },
    restrict: 'E',
    templateUrl: '/static/angular-components/hunt/hunt-inspector.html',
    controller: HuntInspectorController,
    controllerAs: 'controller'
  };
};


/**
 * Directive's name in Angular.
 *
 * @const
 * @export
 */
exports.HuntInspectorDirective.directive_name = 'grrHuntInspector';
