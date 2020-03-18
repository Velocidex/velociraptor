'use strict';

goog.module('grrUi.notebook.notebookListDirective');
goog.module.declareLegacyNamespace();

const {InfiniteTableController} = goog.require('grrUi.core.infiniteTableDirective');

var TABLE_KEY_NAME = InfiniteTableController.UNIQUE_KEY_NAME;
var TABLE_ROW_HASH = InfiniteTableController.ROW_HASH_NAME;


/** @type {number} */
let AUTO_REFRESH_INTERVAL_MS = 5 * 1000;

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

const NotebookListController = function(
    $scope, $element, grrApiService, $uibModal) {
    /** @private {!angular.Scope} */
    this.scope_ = $scope;

    this.uibModal_ = $uibModal;

    /** @private {!angular.jQuery} */
    this.element_ = $element;

    /** @type {!Object<string, Object>} */
    this.notebooksById = {};

    this.grrApiService_ = grrApiService;

    /** @type {?string} */
    this.selectedNotebookId;

    /** @type {function(boolean)} */
    this.triggerTableUpdate;

    /** @type {number} */
    this.autoRefreshInterval = AUTO_REFRESH_INTERVAL_MS;

    /** @type {number} */
    this.pageSize = PAGE_SIZE;

    // Push the selection changes back to the scope, so that other UI components
    // can react on the change.
    this.scope_.$watch('controller.selectedNotebookId', function(newValue, oldValue) {
        // Only propagate real changes, don't propagate initial undefined
        // value.
        if (angular.isDefined(newValue)) {
            this.scope_['selectedNotebookId'] = newValue;
        }
    }.bind(this));

    // Propagate our triggerUpdate implementation to the scope so that users of
    // this directive can use it.
    this.scope_['triggerUpdate'] = this.triggerUpdate.bind(this);
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
NotebookListController.prototype.transformItems = function(items) {
  angular.forEach(items, function(item, index) {
    var last_active_at = 0;
    if (angular.isDefined(item['created_time'])) {
      last_active_at = item['created_time'];
    }

    item[TABLE_KEY_NAME] = item['notebook_id'];
    item[TABLE_ROW_HASH] = [item['notebook_id']];
  }.bind(this));

  return items;
};

NotebookListController.prototype.deleteNotebook = function(event) {
    var selected_notebook_id = this.selectedNotebookId;
    var notebook = this.scope_["state"]["notebook"];
    var self = this;

    notebook.hidden = true;

    this.grrApiService_.post(
        'v1/UpdateNotebook', notebook).then(
            function success(response) {
                self.selectedNotebookId = null;
                self.triggerUpdate();

            }, function failure(response) {
                console.log("Error " + response.data);
            });

    event.stopPropagation();
    return false;
};

/**
 * Selects given item in the list.
 *
 * @param {!Object} item Item to be selected.
 * @export
 */
NotebookListController.prototype.selectItem = function(item) {
    this.selectedNotebookId = item['notebook_id'];
    this.scope_["state"]["notebook"] = item;
};

NotebookListController.prototype.newNotebook = function() {
    var modalScope = this.scope_.$new();
    var self = this;

    var modalInstance = this.uibModal_.open({
        template: '<grr-new-notebook-dialog '+
            'on-resolve="resolve()" on-reject="reject()" />',
        scope: modalScope,
        windowClass: 'wide-modal high-modal',
        size: 'lg'
    });

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

};


/**
 * Triggers a graceful update of the infinite table.
 *
 * @export
 */
NotebookListController.prototype.triggerUpdate = function() {
    this.triggerTableUpdate(true);
};

/**
 * FlowsListDirective definition.

 * @return {angular.Directive} Directive definition object.
 */
exports.NotebookListDirective = function() {
    return {
        scope: {
          selectedNotebookId: '=?',
          state: '=',
        },
        restrict: 'E',
        templateUrl: '/static/angular-components/notebook/notebook-list.html',
        controller: NotebookListController,
    controllerAs: 'controller'
    };
};


/**
 * Directive's name in Angular.
 *
 * @const
 * @export
 */
exports.NotebookListDirective.directive_name = 'grrNotebookList';
