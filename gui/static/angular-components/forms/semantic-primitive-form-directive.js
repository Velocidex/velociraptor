'use strict';

goog.module('grrUi.forms.semanticPrimitiveFormDirective');
goog.module.declareLegacyNamespace();



/**
 * Controller for SemanticPrimitiveFormDirective.
 *
 * @constructor
 * @param {!angular.Scope} $scope
 * @param {!grrUi.core.reflectionService.ReflectionService} grrReflectionService
 * @ngInject
 */
const SemanticPrimitiveFormController =
    function($scope) {
      /** @private {!angular.Scope} */
      this.scope = $scope;

      if (angular.isUndefined(this.scope.value) &&
          angular.isDefined(this.scope['default'])) {
        this.scope.value = JSON.parse(this.scope['default']);
      }
};

/**
 * SemanticPrimitiveFormDirective renders a form for a boolean value.
 *
 * @return {!angular.Directive} Directive definition object.
 */
exports.SemanticPrimitiveFormDirective = function() {
  return {
    restrict: 'E',
    scope: {
      value: '=',
      type: '@',
      default: '=',
    },
    templateUrl: '/static/angular-components/forms/' +
        'semantic-primitive-form.html',
    controller: SemanticPrimitiveFormController,
    controllerAs: 'controller'
  };
};


/**
 * Name of the directive in Angular.
 *
 * @const
 * @export
 */
exports.SemanticPrimitiveFormDirective.directive_name = 'grrFormPrimitive';


/**
 * Semantic type corresponding to this directive.
 *
 * @const
 * @export
 */
exports.SemanticPrimitiveFormDirective.semantic_types = [
  'RDFBool', 'bool',                     // Boolean types.
  'RDFInteger', 'int', 'integer', 'long', 'float',  // Numeric types.
  'RDFString', 'string', 'basestring', 'RDFURN',   // String types.
  'bytes',                               // Byte types.
  // TODO(user): check if we ever have to deal with
  // bytes type (RDFBytes is handled by grr-form-bytes).
];
