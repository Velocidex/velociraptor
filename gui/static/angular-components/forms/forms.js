'use strict';

goog.module('grrUi.forms.forms');
goog.module.declareLegacyNamespace();

const {ClientLabelFormDirective} = goog.require('grrUi.forms.clientLabelFormDirective');
const {DatetimeFormDirective} = goog.require('grrUi.forms.datetimeFormDirective');
const {DatetimeSecondsFormDirective} = goog.require('grrUi.forms.datetimeSecondsFormDirective');

const {DurationFormDirective} = goog.require('grrUi.forms.durationFormDirective');
const {SemanticEnumFormDirective} = goog.require('grrUi.forms.semanticEnumFormDirective');
const {SemanticPrimitiveFormDirective} = goog.require('grrUi.forms.semanticPrimitiveFormDirective');
const {SemanticProtoFormDirective} = goog.require('grrUi.forms.semanticProtoFormDirective');
const {SemanticProtoRepeatedFieldFormDirective} = goog.require('grrUi.forms.semanticProtoRepeatedFieldFormDirective');
const {SemanticProtoSingleFieldFormDirective} = goog.require('grrUi.forms.semanticProtoSingleFieldFormDirective');
const {SemanticProtoUnionFormDirective} = goog.require('grrUi.forms.semanticProtoUnionFormDirective');
const {SemanticRegistryService} = goog.require('grrUi.core.semanticRegistryService');
const {SemanticValueFormDirective} = goog.require('grrUi.forms.semanticValueFormDirective');
const {coreModule} = goog.require('grrUi.core.core');
const {JsonProxyDirective} = goog.require('grrUi.forms.jsonProxyDirective');


/**
 * Angular module for forms-related UI.
 */
exports.formsModule =
    angular.module('grrUi.forms', [coreModule.name, 'ui.bootstrap']);


exports.formsModule.service(
    SemanticRegistryService.forms_service_name, SemanticRegistryService);
exports.formsModule.service(
    SemanticRegistryService.repeated_forms_service_name,
    SemanticRegistryService);


exports.formsModule.directive(
    ClientLabelFormDirective.directive_name, ClientLabelFormDirective);
exports.formsModule.directive(
    DatetimeFormDirective.directive_name, DatetimeFormDirective);
exports.formsModule.directive(
    DatetimeSecondsFormDirective.directive_name, DatetimeSecondsFormDirective);
exports.formsModule.directive(
    DurationFormDirective.directive_name, DurationFormDirective);
exports.formsModule.directive(
    SemanticEnumFormDirective.directive_name, SemanticEnumFormDirective);
exports.formsModule.directive(
    SemanticPrimitiveFormDirective.directive_name,
    SemanticPrimitiveFormDirective);
exports.formsModule.directive(
    SemanticProtoFormDirective.directive_name, SemanticProtoFormDirective);
exports.formsModule.directive(
    SemanticProtoSingleFieldFormDirective.directive_name,
    SemanticProtoSingleFieldFormDirective);
exports.formsModule.directive(
    SemanticProtoRepeatedFieldFormDirective.directive_name,
    SemanticProtoRepeatedFieldFormDirective);
exports.formsModule.directive(
    SemanticProtoUnionFormDirective.directive_name,
    SemanticProtoUnionFormDirective);
exports.formsModule.directive(
    SemanticValueFormDirective.directive_name, SemanticValueFormDirective);
exports.formsModule.directive(
    JsonProxyDirective.directive_name, JsonProxyDirective);

exports.formsModule.run(function(grrSemanticFormDirectivesRegistryService) {
  var registry = grrSemanticFormDirectivesRegistryService;

  registry.registerDirective(
      DatetimeFormDirective.semantic_type, DatetimeFormDirective);

  registry.registerDirective(
      DatetimeSecondsFormDirective.semantic_type, DatetimeSecondsFormDirective);

  registry.registerDirective(
      DurationFormDirective.semantic_type, DurationFormDirective);

  var primitiveSemanticTypes = SemanticPrimitiveFormDirective.semantic_types;
  angular.forEach(primitiveSemanticTypes, function(primitiveSemanticType) {
    registry.registerDirective(
        primitiveSemanticType, SemanticPrimitiveFormDirective);
  });

  registry.registerDirective(
      ClientLabelFormDirective.semantic_type, ClientLabelFormDirective);
  registry.registerDirective(
      SemanticEnumFormDirective.semantic_type, SemanticEnumFormDirective);
  registry.registerDirective(
      SemanticProtoFormDirective.semantic_type, SemanticProtoFormDirective);
});
