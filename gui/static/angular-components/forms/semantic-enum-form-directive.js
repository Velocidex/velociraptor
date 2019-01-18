'use strict';

goog.module('grrUi.forms.semanticEnumFormDirective');
goog.module.declareLegacyNamespace();



/**
 * Controller for SemanticEnumFormDirective.
 *
 * @constructor
 * @param {!angular.Scope} $scope
 * @ngInject
 */
const SemanticEnumFormController = function(
  $scope,   grrReflectionService) {
  /** @private {!angular.Scope} */
  this.scope_ = $scope;

    this.allowedOptions = [];

    /** @type {object} */
    this.valueDescriptor;

  this.grrReflectionService_ = grrReflectionService;

  this.scope_.$watch('value',
                     this.onValueChange_.bind(this));
};


/**
 * Handles changes of the list of allowed values.
 *
 * @param {!Array<Object>} newValue
 * @private
 */
SemanticEnumFormController.prototype.onValueChange_ = function(
  newValue) {
  this.grrReflectionService_.getRDFValueDescriptor(
    this.scope_.type, false, newValue).then(
      function(descriptor) {
        this.valueDescriptor = descriptor;
        if (angular.isUndefined(this.scope_.value)) {
          if (angular.isDefined(this.scope_.default)) {
            this.scope_.value = this.scope_.default;
          } else {
            this.scope_.value = JSON.parse(descriptor.default);
          }
        }
        var self = this;
        this.allowedOptions = [];
        angular.forEach(descriptor.allowed_values, function(value) {
          self.allowedOptions.push(value.name);
        });
      }.bind(this));
};

/**
 * SemanticEnumFormDirective renders an EnumNamedValue.
 *
 * @return {!angular.Directive} Directive definition object.
 */
exports.SemanticEnumFormDirective = function() {
  return {
    restrict: 'E',
    scope: {
      value: '=',
      type: '@',
    },
    templateUrl: '/static/angular-components/forms/semantic-enum-form.html',
    controller: SemanticEnumFormController,
    controllerAs: 'controller'
  };
};


/**
 * Name of the directive in Angular.
 *
 * @const
 * @export
 */
exports.SemanticEnumFormDirective.directive_name = 'grrFormEnum';

/**
 * Semantic type corresponding to this directive.
 *
 * @const
 * @export
 */
exports.SemanticEnumFormDirective.semantic_type = 'EnumNamedValue';
