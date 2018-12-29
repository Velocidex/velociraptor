'use strict';

goog.module('grrUi.forms.semanticValueFormDirective');
goog.module.declareLegacyNamespace();

const {debug} = goog.require('grrUi.core.utils');


/**
 * Controller for SemanticValueFormDirective.
 *
 * @constructor
 * @param {!angular.Scope} $scope
 * @param {!angular.$compile} $compile
 * @param {!angular.jQuery} $element
 * @param {!grrUi.core.reflectionService.ReflectionService} grrReflectionService
 * @ngInject
 */
exports.SemanticValueFormController = function(
  $scope, $compile, $element,
  grrReflectionService,
  grrSemanticFormDirectivesRegistryService) {
  /** @private {!angular.Scope} */
  this.scope_ = $scope;

  /** @private {!angular.$compile} */
  this.compile_ = $compile;

  /** @private {!grrUi.core.reflectionService.ReflectionService} */
  this.grrReflectionService_ = grrReflectionService;
  this.grrSemanticFormDirectivesRegistryService_ = grrSemanticFormDirectivesRegistryService;

  debug("SemanticValueFormController", this.scope_.value);

  // The descriptor of the proto we are trying to render.
  this.valueDescriptor;
  this.renderer;

  this.scope_.$watch('value', this.onValueChange_.bind(this));
};

var SemanticValueFormController = exports.SemanticValueFormController;

SemanticValueFormController.prototype.onValueChange_ = function(newValue, oldValue) {
  if (angular.isUndefined(newValue) && angular.isUndefined(this.scope_.type)) {
    console.log("Error - no type provided for semantic-value-form-directive.");
    return;
  }

  /**
   * Previous versions of this code had both editedValue and value
   * objects in order to avoid copying defaults to the value. However
   * in proto3 there are no defaults so we actually do want to copy
   * our defaults into the value which is sent - otherwise these
   * defaults will not be set at all by the server.
   */
  if (1 || newValue !== oldValue) {
    this.grrReflectionService_.getRDFValueDescriptor(
      this.scope_.type, false, newValue).then(
        function(descriptor) {
            this.valueDescriptor = descriptor;
          var directive = this.grrSemanticFormDirectivesRegistryService_.findDirectiveForDescriptor(
              descriptor);
          this.renderer = directive.directive_name;
        }.bind(this));
  }
};

SemanticValueFormController.prototype.typeOfValue_ = function(value) {
  var type = this.scope_.type;

  // Any values get their real type from the value itself.
  if (type == ".google.protobuf.Any") {
    var prefix = "type.googleapis.com/proto.";
    type = value['@type'];
    if (angular.isUndefined(type)) {
      debugger;
    }

    if (type.startsWith(prefix)) {
      type = type.slice(prefix.length);
    }
  }

  return type;
};

/**
 * SemanticValueFormDirective renders a form corresponding to a given semantic
 * RDF type.
 *
 * @return {!angular.Directive} Directive definition object.
 */
exports.SemanticValueFormDirective = function() {
  return {
    restrict: 'E',
    scope: {
      value: '=',
      default: '=',
      type: '@',
    },
    templateUrl: '/static/angular-components/forms/semantic-value-form.html',
    controller: SemanticValueFormController,
    controllerAs: 'controller'
  };
};


/**
 * Name of the directive in Angular.
 *
 * @const
 * @export
 */
exports.SemanticValueFormDirective.directive_name = 'grrFormValue';
