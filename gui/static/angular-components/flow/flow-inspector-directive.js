'use strict';

goog.module('grrUi.flow.flowInspectorDirective');
goog.module.declareLegacyNamespace();


/** @type {number} */
let AUTO_REFRESH_INTERVAL_MS = 1500 * 1000;

/**
 * Sets the delay between automatic refreshes of the flow overview.
 *
 * @param {number} millis Interval value in milliseconds.
 * @export
 */
exports.setAutoRefreshInterval = function(millis) {
  AUTO_REFRESH_INTERVAL_MS = millis;
};

/**
 * Controller for FlowInspectorDirective.
 *
 * @constructor
 * @param {!angular.Scope} $scope
 * @ngInject
 */
const FlowInspectorController = function($scope, grrApiService) {
  /** @private {!angular.Scope} */
  this.scope_ = $scope;

  /** @type {string} */
  this.activeTab = '';

  /** @private {!grrUi.core.apiService.ApiService} */
  this.grrApiService_ = grrApiService;

  /** type {Object<string, boolean>} */
  this.tabsShown = {};

  this.scope_.$watch('activeTab', this.onDirectiveArgumentsChange_.bind(this));
    this.scope_.$watch('controller.activeTab', this.onTabChange_.bind(this));

  this.scope_.$watchGroup(['flowId'],
                          this.startPolling.bind(this));

  /** @export {Object} */
  this.flow;

  /** @private {!angular.$q.Promise|undefined} */
  this.pollPromise_;

  this.scope_.$on('$destroy', function() {
    this.grrApiService_.cancelPoll(this.pollPromise_);
  }.bind(this));
};


/**
 * Start polling for flow data.
 *
 * @export
 */
FlowInspectorController.prototype.startPolling = function(newValues, oldValues) {
    this.grrApiService_.cancelPoll(this.pollPromise_);
    this.pollPromise_ = undefined;

  if (angular.isDefined(this.scope_['apiBasePath']) &&
      angular.isDefined(this.scope_['flowId'])) {
    var flowUrl = this.scope_['apiBasePath'];
    var interval = AUTO_REFRESH_INTERVAL_MS;

      this.flow = null;

    // It's important to assign the result of the poll() call, not the
    // result of the poll().then() call, since we need the original
    // promise to pass to cancelPoll if needed.
    this.pollPromise_ = this.grrApiService_.poll(
      flowUrl, interval, {flow_id: this.scope_['flowId']});
    this.pollPromise_.then(
        undefined,
        undefined,
        function notify(response) {
          this.flow = response['data'];
        }.bind(this));
  }
};


/**
 * Handles changes to directive's arguments.'
 *
 * @param {string} newValue
 * @private
 */
FlowInspectorController.prototype.onDirectiveArgumentsChange_ = function(newValue) {
  if (angular.isString(newValue)) {
    this.activeTab = newValue;
  }
};

/**
 * Downloades the file.
 *
 * @export
 */
FlowInspectorController.prototype.downloadFile = function() {
  var clientId = this.flow['client_id'];
  var flow_id = this.flow['flow_id'];
  var filename = clientId + '/' + flow_id;

  // Sanitize filename for download.
  var url = 'v1/download/' + filename.replace(/[^./a-zA-Z0-9]+/g, '_');
  this.grrApiService_.downloadFile(url).then(
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
 * Handles controller's activeTab attribute changes and propagates them to the
 * directive's scope.
 *
 * @param {string} newValue
 * @param {string} oldValue
 * @private
 */
FlowInspectorController.prototype.onTabChange_ = function(newValue, oldValue) {
  if (newValue !== oldValue) {
    this.scope_['activeTab'] = newValue;
  }
  this.tabsShown[newValue] = true;
};


/**
 * FlowInspectorDirective definition.

 * @return {angular.Directive} Directive definition object.
 */
exports.FlowInspectorDirective = function() {
  return {
    scope: {
      flowId: '=',
      apiBasePath: '=',
      activeTab: '=?',
      exportBasePath: '=',
    },
    controller: FlowInspectorController,
    controllerAs: 'controller',
    restrict: 'E',
    templateUrl: '/static/angular-components/flow/flow-inspector.html'
  };
};


/**
 * Directive's name in Angular.
 *
 * @const
 * @export
 */
exports.FlowInspectorDirective.directive_name = 'grrFlowInspector';
