'use strict';

goog.module('grrUi.flow.flowsListDirective');
goog.module.declareLegacyNamespace();

const {InfiniteTableController} = goog.require('grrUi.core.infiniteTableDirective');



var TABLE_KEY_NAME = InfiniteTableController.UNIQUE_KEY_NAME;
var TABLE_ROW_HASH = InfiniteTableController.ROW_HASH_NAME;


/** @type {number} */
let AUTO_REFRESH_INTERVAL_MS = 30 * 1000;

/**
 * Sets the delay between automatic refreshes of the flow list.
 *
 * @param {number} millis Interval value in milliseconds.
 * @export
 */
exports.setAutoRefreshInterval = function(millis) {
  AUTO_REFRESH_INTERVAL_MS = millis;
};


/** @const {number} */
const PAGE_SIZE = 100;


/**
 * Controller for FlowsListDirective.
 *
 * @constructor
 * @param {!angular.Scope} $scope
 * @param {!angular.jQuery} $element
 * @param {!grrUi.core.apiService.ApiService} grrApiService
 * @ngInject
 */
const FlowsListController = function(
    $scope, $element, grrApiService, grrRoutingService) {
  /** @private {!angular.Scope} */
    this.scope_ = $scope;

    this.grrRoutingService_ = grrRoutingService;

  /** @private {!angular.jQuery} */
  this.element_ = $element;

  /** @type {!Object<string, Object>} */
  this.flowsById = {};

  /** @type {?string} */
  this.selectedFlowId;

  /** @type {function(boolean)} */
  this.triggerTableUpdate;

  /** @type {number} */
  this.autoRefreshInterval = AUTO_REFRESH_INTERVAL_MS;

  /** @type {number} */
  this.pageSize = PAGE_SIZE;

  // Push the selection changes back to the scope, so that other UI components
  // can react on the change.
  this.scope_.$watch('controller.selectedFlowId', function(newValue, oldValue) {
    // Only propagate real changes, don't propagate initial undefined
    // value.
    if (angular.isDefined(newValue)) {
      this.scope_['selectedFlowId'] = newValue;
    }
  }.bind(this));

  // Propagate our triggerUpdate implementation to the scope so that users of
  // this directive can use it.
    this.scope_['triggerUpdate'] = this.triggerUpdate.bind(this);

    this.grrRoutingService_.uiOnParamsChanged(
        this.scope_, ['clientId', 'flowId'],
        this.onRoutingParamsChange_.bind(this));
};


FlowsListController.prototype.onRoutingParamsChange_ = function(
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
 * Transforms items fetched by API items provider. The
 * InfiniteTableController requires a unique key per row so it may be
 * updated.
 *
 * @param {!Array<Object>} items Items to be transformed.
 * @return {!Array<Object>} Transformed items.
 * @export
 */
FlowsListController.prototype.transformItems = function(items) {
  angular.forEach(items, function(item, index) {
    var state = 'BROKEN';
    if (angular.isDefined(item['state'])) {
      state = item['state'];
    }

    var last_active_at = 0;
    if (angular.isDefined(item['create_time'])) {
      last_active_at = item['create_time'];
    }

    item[TABLE_KEY_NAME] = item['session_id'];
    item[TABLE_ROW_HASH] = [state, last_active_at];
  }.bind(this));

  return items;
};

/**
 * Selects given item in the list.
 *
 * @param {!Object} item Item to be selected.
 * @export
 */
FlowsListController.prototype.selectItem = function(item) {
  this.selectedFlowId = item['session_id'];
  this.scope_['selectedFlowState'] = item['state'];
};

/**
 * Triggers a graceful update of the infinite table.
 *
 * @export
 */
FlowsListController.prototype.triggerUpdate = function() {
    this.triggerTableUpdate(true);
};

/**
 * FlowsListDirective definition.

 * @return {angular.Directive} Directive definition object.
 */
exports.FlowsListDirective = function() {
    return {
        scope: {
          flowsUrl: '=',
          selectedFlowId: '=?',
          selectedFlowState: '=?',
          triggerUpdate: '=?',
          clientId: '=',
        },
        restrict: 'E',
        templateUrl: window.base_path+'/static/angular-components/flow/flows-list.html',
        controller: FlowsListController,
    controllerAs: 'controller'
    };
};


/**
 * Directive's name in Angular.
 *
 * @const
 * @export
 */
exports.FlowsListDirective.directive_name = 'grrFlowsList';
