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
const SemanticProtoUnionFormController = function(
  $scope, $attrs, grrReflectionService) {
  /** @private {!angular.Scope} */
  this.scope_ = $scope;

  /** @type {Object|undefined} */
  this.unionField;

  this.cache = {};

  /** @private {!grrUi.core.reflectionService.ReflectionService} */
  this.grrReflectionService_ = grrReflectionService;

  this.valueDescriptor;

  $scope.$watch('controller.unionField',
                this.onUnionFieldChange_.bind(this));

  if (angular.isDefined(this.scope_.value)) {
    var self = this;
    angular.forEach(this.scope_.field['fields'], function(field) {
      if (angular.isDefined(self.scope_.value[field['name']])) {
        self.unionField = field;
      };
    });
  }

};

SemanticProtoUnionFormController.prototype.onUnionFieldChange_ = function(
  newValue, oldValue) {
    if (newValue == oldValue) {
        return;
    }

    var value = this.scope_.value;

  if (angular.isUndefined(newValue)) {
    return;
  }

  var name = newValue.name;

  var existing_field = this.cache[name];
  if (angular.isUndefined(existing_field)) {
    if (angular.isDefined(newValue['default'])) {
      existing_field = JSON.parse(newValue.default);
    } else {
      existing_field = {};
    }
  }

  Object.keys(value).forEach(function(key) { delete value[key]; });
  value[name] = existing_field;
  this.cache[newValue.name] = existing_field;

  self.unionField = newValue;
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
