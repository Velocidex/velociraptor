'use strict';

goog.module('grrUi.hunt.huntInspectorDirective');
goog.module.declareLegacyNamespace();

/** @type {number} */
let AUTO_REFRESH_INTERVAL_MS = 5 * 1000;

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
    $scope, grrApiService, grrRoutingService, grrAceService) {
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

    this.hunt = {};

    /** @private {!angular.$q.Promise|undefined} */
    this.pollPromise_;

    this.scope_.$on('$destroy', function() {
        this.grrApiService_.cancelPoll(this.pollPromise_);
    }.bind(this));

    this.scope_.$watch('huntId', this.startPolling_.bind(this));

    this.serializedRequests = "";

    var self = this;

    this.scope_.aceConfig = function(ace) {
        self.ace = ace;
        grrAceService.AceConfig(ace);

        ace.setOptions({
            wrap: true,
            autoScrollEditorIntoView: true,
            minLines: 10,
            maxLines: 100,
        });

        self.scope_.$on('$destroy', function() {
            grrAceService.SaveAceConfig(ace);
        });

        ace.resize();
    };
};

HuntInspectorController.prototype.showSettings = function() {
    this.ace.execCommand("showSettingsMenu");
};

/**
 * Fetches hunt data;
 *
 * @private
 */
HuntInspectorController.prototype.startPolling_ = function() {
    var self = this;

    self.grrApiService_.cancelPoll(self.pollPromise_);
    self.pollPromise_ = undefined;

    if (angular.isDefined(self.scope_['huntId'])) {
        var huntId = self.scope_['huntId'];
        self.pollPromise_ = self.grrApiService_.poll(
            'v1/GetHunt',
            AUTO_REFRESH_INTERVAL_MS,
            {hunt_id: huntId});
        self.pollPromise_.then(
            undefined,
            undefined,
            function notify(response) {
                self.hunt = response['data'];
                if (angular.isObject(self.hunt.start_request.compiled_collector_args)) {
                    self.serializedRequests = JSON.stringify(
                        self.hunt.start_request.compiled_collector_args, null, 4);
                }
            });
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
