'use strict';

goog.module('grrUi.forms.semanticProtoFormDirective');
goog.module.declareLegacyNamespace();



/**
 * Controller for SemanticProtoFormDirective.
 *
 * @constructor
 * @param {!angular.Scope} $scope
 * @param {!angular.Attributes} $attrs
 * @param {!grrUi.core.reflectionService.ReflectionService} grrReflectionService
 * @ngInject
 */
const SemanticProtoFormController = function(
    $scope, $attrs, grrReflectionService) {
  /** @private {!angular.Scope} */
  this.scope_ = $scope;

  /** @type {?} */
  this.scope_.value;

  this.scope_.type;

  /** @private {!grrUi.core.reflectionService.ReflectionService} */
  this.grrReflectionService_ = grrReflectionService;

  /** @type {boolean} */
  this.advancedShown = false;

  /** @type {boolean} */
  this.hasAdvancedFields = false;

  /** @type {boolean} */
  this.expanded = false;

  if (angular.isDefined($attrs['hiddenFields']) &&
      angular.isDefined($attrs['visibleFields'])) {
    throw new Error('Either hidden-fields or visible-fields attribute may ' +
                    'be specified, not both.');
  }

  this.scope_.$watch('value', this.onValueChange_.bind(this));
  this.boundNotExplicitlyHiddenFields =
      this.notExplicitlyHiddenFields_.bind(this);
};

SemanticProtoFormController.prototype.typeOfValue_ = function(value) {
  if (angular.isUndefined(value)) {
    return "string";
  }

  var type = value['@type'];

  if (angular.isDefined(type)) {
    var prefix = "type.googleapis.com/proto.";
    if (type.startsWith(prefix)) {
      type = type.slice(prefix.length);
    }

    return type;
  }

  return this.scope_.type;
};


/**
 * Filter function that returns true if the field wasn't explicitly mentioned
 * in 'hidden-fields' directive's argument.
 *
 * @param {string} field Name of a field.
 * @param {number=} opt_index Index of the field name in the names list
 *                            (optional).
 * @return {boolean} True if the field is not hidden, false otherwise.
 * @private
 */
SemanticProtoFormController.prototype.notExplicitlyHiddenFields_ = function(
    field, opt_index) {
  if (angular.isDefined(this.scope_['hiddenFields'])) {
    return this.scope_['hiddenFields'].indexOf(field['name']) == -1;
  } else if (angular.isDefined(this.scope_['visibleFields'])) {
    return this.scope_['visibleFields'].indexOf(field['name']) != -1;
  } else {
    return true;
  }
};

/**
 * Predicate that returns true only for regular (non-hidden, non-advanced)
 * fields.
 *
 * @param {Object} field Descriptor field to check.
 * @param {Number} index Descriptor field index.
 * @return {boolean}
 * @export
 */
SemanticProtoFormController.prototype.regularFieldsOnly = function(
    field, index) {
  return angular.isUndefined(field.labels) ||
      field.labels.indexOf('HIDDEN') == -1 &&
      field.labels.indexOf('ADVANCED') == -1;
};


/**
 * Predicate that returns true only for advanced (and non-hidden) fields.
 *
 * @param {Object} field Descriptor field to check.
 * @param {Number} index Descriptor field index.
 * @return {boolean}
 * @export
 */
SemanticProtoFormController.prototype.advancedFieldsOnly = function(
    field, index) {
  return angular.isDefined(field.labels) &&
      field.labels.indexOf('HIDDEN') == -1 &&
      field.labels.indexOf('ADVANCED') != -1;
};


/**
 * Handles changes of the value type.
 *
 * @param {?string} newValue
 * @param {?string} oldValue
 * @private
 */
SemanticProtoFormController.prototype.onValueChange_ = function(
    newValue, oldValue) {
  if (angular.isUndefined(newValue) && angular.isUndefined(this.scope_.type)) {
    console.log("Error - no type provided for semantic-value-form-directive.");
    return;
  }

  this.scope_.type = this.typeOfValue_(newValue);

  /**
   * Previous versions of this code had both editedValue and value
   * objects in order to avoid copying defaults to the value. However
   * in proto3 there are no defaults so we actually do want to copy
   * out defaults into the value which is sent - otherwise these
   * defaults will not be set at all by the server.
   */
  if (newValue !== oldValue || angular.isUndefined(this.valueDescriptor)) {
    this.grrReflectionService_.getRDFValueDescriptor(
        this.scope_.type, true).then(
            this.onDescriptorsFetched_.bind(this));
  }
};


/**
 * Handles fetched reflection data.
 *
 * @param {!Object<string, Object>} descriptors
 * @private
 */
SemanticProtoFormController.prototype.onDescriptorsFetched_ = function(
    descriptors) {
  var self = this;
  this.descriptors = descriptors;
  this.valueDescriptor = angular.copy(descriptors[this.scope_.type]);

  // Oneof fields never hold any defaults.
  if (this.valueDescriptor.oneof) {
    return;
  }

  angular.forEach(this.valueDescriptor['fields'], function(field) {
    if (angular.isDefined(field.labels)) {
      if (field.labels.indexOf('HIDDEN') != -1) {
        return;
      }

      if (field.labels.indexOf('ADVANCED') != -1) {
        self.hasAdvancedFields = true;
      }
    }

    if (field.repeated) {
      field.depth = 0;

      if (angular.isUndefined(this.scope_.value[field.name])) {
        this.scope_.value[field.name] = [];
      }
    } else {
      field.depth = (this.scope_.$eval('metadata.depth') || 0) + 1;

      if (angular.isUndefined(this.scope_.value[field.name])) {
        if (angular.isDefined(field['default'])) {
          this.scope_.value[field.name] = JSON.parse(field['default']);
        } else {
          if (field.kind == 'primitive') {
            this.scope_.value[field.name] = "";
          } else {
            this.scope_.value[field.name] = {};
          }
        }
      }
    }
  }.bind(this));
};

/**
 * SemanticProtoFormDirective renders a form corresponding to a given
 * RDFProtoStruct.
 *
 * @return {!angular.Directive} Directive definition object.
 */
exports.SemanticProtoFormDirective = function() {
  return {
    scope: {
      value: '=',
      type: '@',
      metadata: '=?',
      hiddenFields: '=?',
      visibleFields: '=?'
    },
    restrict: 'E',
    templateUrl: '/static/angular-components/forms/semantic-proto-form.html',
    controller: SemanticProtoFormController,
    controllerAs: 'controller'
  };
};


/**
 * Name of the directive in Angular.
 *
 * @const
 * @export
 */
exports.SemanticProtoFormDirective.directive_name = 'grrFormProto';


/**
 * Semantic type corresponding to this directive.
 *
 * @const
 * @export
 */
exports.SemanticProtoFormDirective.semantic_type = 'RDFProtoStruct';
