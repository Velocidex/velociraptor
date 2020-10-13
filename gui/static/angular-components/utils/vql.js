'use strict';

goog.module('grrUi.utils.vqlDirective');

/**
 * Controller for VQLDirective.
 *
 * @param {!angular.Scope} $scope
 * @param {!angular.jQuery} $element
 * @constructor
 * @ngInject
 */
const vqlController = function(
    $scope, $element) {
    /** @private {!angular.Scope} */
    this.scope_ = $scope;

    /** @type {?} */
    this.scope_.value;

    /** @type {object} */
    this.payload;
    /** @type {object} */
    this.columns;

    /** @type {object} */
    this.type_map = {};

    /** @type {string} */
    this.query = "";
    this.selectedIndex = -1;

    /** @private {!angular.jQuery} $element */
    this.element_ = $element;

    this.scope_.$watch('::value', this.onValueChange.bind(this));
};



/**
 * Handles changes of scope.value attribute.
 *
 * @param {number} newValue VQLResponse from client.
 * @suppress {missingProperties} as value can be anything.
 */
vqlController.prototype.onValueChange = function(newValue) {
  if (angular.isDefined(newValue)) {
    this.columns = [];
    this.type_map = {};

    if (angular.isDefined(newValue['types'])) {
      for (var i = 0; i < newValue['types'].length; i++) {
        var type = newValue.types[i];
        this.type_map[type['column']] = type['type'];
      }
    }

    this.payload = JSON.parse(newValue.Response);

    var columns = [];
    if (angular.isDefined(newValue.Columns)) {
      columns = newValue.Columns;
    }

    if (columns.length == 0 && this.payload.length > 0) {
      // Sorting to get some stable order.
      columns = Object.keys(this.payload[0]).sort();
    }

    // Hide columns beginning with _ from the table.
    for (var i = 0; i < columns.length; i++) {
      var column = columns[i];
      if (!column.startsWith("_")) {
        this.columns.push(column);
      }
    }

    this.value =  newValue;
  }
};


vqlController.prototype.selectRow_ = function(row, index) {
    this.scope_.selectedRow = row;
    this.selectedIndex = index;
};


/**
 * Directive that displays VQLResponse values.
 *
 * @return {!angular.Directive} Directive definition object.
 * @ngInject
 * @export
 */
exports.VQLDirective = function() {
  return {
    scope: {
      value: '=',
      selectedRow: '=?',
    },
    restrict: 'E',
    templateUrl: window.base_path+'/static/angular-components/utils/vql.html',
    controller: vqlController,
    controllerAs: 'controller'
  };
};


/**
 * Directive's name in Angular.
 *
 * @const
 * @export
 */
exports.VQLDirective.directive_name = 'grrVql';
