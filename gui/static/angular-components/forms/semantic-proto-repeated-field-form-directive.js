'use strict';

goog.module('grrUi.forms.semanticProtoRepeatedFieldFormDirective');
goog.module.declareLegacyNamespace();

const {camelCaseToDashDelimited} = goog.require('grrUi.core.utils');
const {debug} = goog.require('grrUi.core.utils');

/**
 * Controller for SemanticProtoRepeatedFieldFormDirective.
 *
 * @constructor
 * @param {!angular.Scope} $scope
 * @param {!angular.jQuery} $element
 * @param {!grrUi.core.semanticRegistryService.SemanticRegistryService}
 *     grrSemanticRepeatedFormDirectivesRegistryService
 * @ngInject
 */
const SemanticProtoRepeatedFieldFormController = function(
        $scope, $element, grrReflectionService) {
  /** @private {!angular.Scope} */
  this.scope_ = $scope;

  /** @private {!angular.jQuery} */
  this.element_ = $element;

  /** @private {!grrUi.core.reflectionService.ReflectionService} */
  this.grrReflectionService_ = grrReflectionService;

    // The descriptor of the proto we are trying to render.
    /** @type {object} */
  this.valueDescriptor;

  this.scope_.$watch('value', this.onValueChange_.bind(this));
  if (angular.isUndefined(this.scope_.value)) {
    if (angular.isDefined(this.scope_.field['default'])) {
      this.scope_.value = JSON.parse(this.scope_.field['default']);
    } else {
      this.scope_.value = [];
    }
  }

  debug("SemanticProtoRepeatedFieldFormController", this.scope_.value);
};


SemanticProtoRepeatedFieldFormController.prototype.onValueChange_ = function(
    newValue, oldValue) {
  /**
   * Previous versions of this code had both editedValue and value
   * objects in order to avoid copying defaults to the value. However
   * in proto3 there are no defaults so we actually do want to copy
   * our defaults into the value which is sent - otherwise these
   * defaults will not be set at all by the server.
   */
  this.grrReflectionService_.getRDFValueDescriptor(
    this.scope_.field['type'], false, newValue).then(
      function(descriptor) {
        this.valueDescriptor = descriptor;
      }.bind(this));
};

/**
 * Handles clicks on 'Add' button.
 *
 * @export
 */
SemanticProtoRepeatedFieldFormController.prototype.addItem = function() {
  var newItem = {};
  if (angular.isDefined(this.valueDescriptor['default'])) {
    newItem = JSON.parse(this.valueDescriptor.default);
  }

  this.scope_.value.splice(0, 0, newItem);
};


/**
 * Handles clicks on 'Remove' buttons.
 *
 * @param {number} index Index of the element to remove.
 * @export
 */
SemanticProtoRepeatedFieldFormController.prototype.removeItem = function(
    index) {
  this.scope_.value.splice(index, 1);
};


/**
 * SemanticProtoRepeatedFieldFormDirective renders a form corresponding to a
 * repeated field of a RDFProtoStruct.
 *
 * @return {!angular.Directive} Directive definition object.
 */
exports.SemanticProtoRepeatedFieldFormDirective = function() {
  return {
    scope: {
      value: '=',
      field: '=',
      noCustomTemplate: '='
    },
    restrict: 'E',
    templateUrl: '/static/angular-components/forms/' +
        'semantic-proto-repeated-field-form.html',
    controller: SemanticProtoRepeatedFieldFormController,
    controllerAs: 'controller'
  };
};


/**
 * Name of the directive in Angular.
 *
 * @const
 * @export
 */
exports.SemanticProtoRepeatedFieldFormDirective.directive_name =
    'grrFormProtoRepeatedField';
