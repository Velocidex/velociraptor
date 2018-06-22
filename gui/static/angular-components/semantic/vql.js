'use strict';

goog.module('grrUi.semantic.vqlDirective');
goog.module.declareLegacyNamespace();

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

  this.payload;
  this.columns;

  this.query = "";

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
    for (var i=0; i<newValue.Columns.length; i++) {
      this.columns.push(newValue.Columns[i]);
    }
    this.payload = JSON.parse(newValue.Response);
    this.query =  newValue.Query;
    console.log(newValue);
  }
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
      payload: '=',
      columns: '='
    },
    restrict: 'E',
    templateUrl: '/static/angular-components/semantic/vql.html',
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

/**
 * Semantic type corresponding to this directive.
 *
 * @const
 * @export
 */
exports.VQLDirective.semantic_type = 'VQLResponse';
