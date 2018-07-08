'use strict';

goog.module('grrUi.forms.semanticProtoUnionFormDirective');
goog.module.declareLegacyNamespace();


/**
 * Controller for SemanticProtoUnionFormDirective.
 *
 * @constructor
 * @param {!angular.Scope} $scope
 * @ngInject
 */
const SemanticProtoUnionFormController = function($scope) {
  /** @private {!angular.Scope} */
  this.scope_ = $scope;

  /** @type {Object|undefined} */
  this.unionField;

  $scope.$watch('controller.unionField',
                this.onUnionFieldChange_.bind(this));
//  $scope.$watch('value[controller.unionField.name]',
//                this.onUnionFieldValueChange_.bind(this));
};


SemanticProtoUnionFormController.prototype.onUnionFieldChange_ = function(
  newValue, oldValue) {
  var value = this.scope_.value;
  var self = this;

  if (angular.isDefined(newValue)) {
    var existing_field = this.scope_.value[newValue.name] || {};

    Object.keys(value).forEach(function(key) { delete value[key]; });
    if (angular.isUndefined(value[newValue.name])) {
      value[newValue.name] = existing_field;
    }
    self.unionField = newValue;
  } else if (angular.isDefined(value)) {
      angular.forEach(self.scope_.field['fields'], function(field) {
        if (angular.isDefined(value[field.name] && angular.isUndefined(self.unionField))) {
          self.unionField = field;
        }
      });
  };
};

/**
* Handles changes of the union field value.
*
* @param {?string} newValue
* @param {?string} oldValue
* @private
*/
SemanticProtoUnionFormController.prototype.onUnionFieldValueChange_ = function(
    newValue, oldValue) {
  if (angular.isDefined(newValue)) {
    if (angular.isDefined(oldValue) &&
        oldValue !== newValue) {
      var unionPart = this.scope_['value'][this.unionFieldValue];

      if (angular.isObject(unionPart)) {
        // We have to make sure that we replace the object at
        // value.value[controller.unionFieldValue]
        unionPart['value'] = {};
        this.scope_['value'][this.unionFieldValue] =
            angular.copy(unionPart);
      }
    }

    this.unionFieldValue = newValue.toLowerCase();
  } else {
    this.unionFieldValue = undefined;
  }
};


/**
 * SemanticProtoUnionFormDirective renders a form corresponding to a
 * an RDFProtoStruct with a union field. This kind of RDFProtoStructs behave
 * similarly to C union types. They have a type defined by the union field.
 * This type determines which nested structure is used/inspected.
 *
 * @return {!angular.Directive} Directive definition object.
 */
exports.SemanticProtoUnionFormDirective = function() {
  return {
    scope: {
      value: '=',
      field: '=',
    },
    restrict: 'E',
    templateUrl: '/static/angular-components/forms/' +
        'semantic-proto-union-form.html',
    controller: SemanticProtoUnionFormController,
    controllerAs: 'controller'
  };
};


/**
 * Name of the directive in Angular.
 *
 * @const
 * @export
 */
exports.SemanticProtoUnionFormDirective.directive_name = 'grrFormProtoUnion';
