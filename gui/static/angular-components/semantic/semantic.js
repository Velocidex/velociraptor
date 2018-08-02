'use strict';

goog.module('grrUi.semantic.semantic');
goog.module.declareLegacyNamespace();

const {ApiHuntResultDirective} = goog.require('grrUi.semantic.apiHuntResultDirective');
const {ByteSizeDirective} = goog.require('grrUi.semantic.byteSizeDirective');
const {ClientUrnDirective} = goog.require('grrUi.semantic.clientUrnDirective');
const {DictDirective} = goog.require('grrUi.semantic.dictDirective');
const {DurationDirective} = goog.require('grrUi.semantic.durationDirective');
const {FlowIdDirective} = goog.require('grrUi.semantic.flowIdDirective');
const {HashDigestDirective} = goog.require('grrUi.semantic.hashDigestDirective');
const {HashListDirective} = goog.require('grrUi.semantic.hashListDirective');
const {HuntIdDirective} = goog.require('grrUi.semantic.huntIdDirective');
const {JsonDirective} = goog.require('grrUi.semantic.jsonDirective');
const {MacAddressDirective} = goog.require('grrUi.semantic.macAddressDirective');
const {NetworkAddressDirective} = goog.require('grrUi.semantic.networkAddressDirective');
const {ObjectLabelDirective} = goog.require('grrUi.semantic.objectLabelDirective');
const {ObjectLabelsListDirective} = goog.require('grrUi.semantic.objectLabelsListDirective');
const {PrimitiveDirective} = goog.require('grrUi.semantic.primitiveDirective');
const {RegistryOverrideDirective, SemanticValueDirective} = goog.require('grrUi.semantic.semanticValueDirective');
const {SemanticProtoDirective} = goog.require('grrUi.semantic.semanticProtoDirective');
const {SemanticRegistryService} = goog.require('grrUi.core.semanticRegistryService');
const {TimestampDirective} = goog.require('grrUi.semantic.timestampDirective');
const {TimestampSecondsDirective} = goog.require('grrUi.semantic.timestampSecondsDirective');
const {UrnDirective} = goog.require('grrUi.semantic.urnDirective');
const {VQLDirective} = goog.require('grrUi.semantic.vqlDirective');
const {coreModule} = goog.require('grrUi.core.core');
const {pseudoModule} = goog.require('grrUi.semantic.pseudo.pseudo');
const {routingModule} = goog.require('grrUi.routing.routing');

/**
 * Module with directives that render semantic values (i.e. RDFValues) fetched
 * from the server.
 */
exports.semanticModule = angular.module('grrUi.semantic', [
  coreModule.name, routingModule.name, pseudoModule.name,
  'ui.bootstrap'
]);

exports.semanticModule.directive(
    ApiHuntResultDirective.directive_name, ApiHuntResultDirective);
exports.semanticModule.directive(
    ByteSizeDirective.directive_name, ByteSizeDirective);
exports.semanticModule.directive(
    ClientUrnDirective.directive_name, ClientUrnDirective);
exports.semanticModule.directive(DictDirective.directive_name, DictDirective);
exports.semanticModule.directive(
    DurationDirective.directive_name, DurationDirective);
exports.semanticModule.directive(
    FlowIdDirective.directive_name, FlowIdDirective);
exports.semanticModule.directive(
    HashDigestDirective.directive_name, HashDigestDirective);
exports.semanticModule.directive(
    HashListDirective.directive_name, HashListDirective);
exports.semanticModule.directive(
    HuntIdDirective.directive_name, HuntIdDirective);
exports.semanticModule.directive(JsonDirective.directive_name, JsonDirective);
exports.semanticModule.directive(
  MacAddressDirective.directive_name, MacAddressDirective);
exports.semanticModule.directive(
    NetworkAddressDirective.directive_name, NetworkAddressDirective);
exports.semanticModule.directive(
    ObjectLabelDirective.directive_name, ObjectLabelDirective);
exports.semanticModule.directive(
    ObjectLabelsListDirective.directive_name, ObjectLabelsListDirective);
exports.semanticModule.directive(
    PrimitiveDirective.directive_name, PrimitiveDirective);
exports.semanticModule.directive(
  SemanticProtoDirective.directive_name, SemanticProtoDirective);
exports.semanticModule.directive(
  RegistryOverrideDirective.directive_name, RegistryOverrideDirective);
exports.semanticModule.directive(
  SemanticValueDirective.directive_name, SemanticValueDirective);
exports.semanticModule.directive(
    TimestampDirective.directive_name, TimestampDirective);
exports.semanticModule.directive(
    TimestampSecondsDirective.directive_name, TimestampSecondsDirective);
exports.semanticModule.directive(UrnDirective.directive_name, UrnDirective);
exports.semanticModule.directive(
  VQLDirective.directive_name, VQLDirective);

exports.semanticModule.service(
    SemanticRegistryService.values_service_name, SemanticRegistryService);

exports.semanticModule.run(function(grrSemanticValueDirectivesRegistryService) {
  var registry = grrSemanticValueDirectivesRegistryService;

  registry.registerDirective(
      ApiHuntResultDirective.semantic_type, ApiHuntResultDirective);
  registry.registerDirective(
      ByteSizeDirective.semantic_type, ByteSizeDirective);
  angular.forEach(ClientUrnDirective.semantic_types, function(type) {
    registry.registerDirective(type, ClientUrnDirective);
  }.bind(this));
  angular.forEach(DictDirective.semantic_types, function(type) {
    registry.registerDirective(type, DictDirective);
  }.bind(this));
  registry.registerDirective(
      DurationDirective.semantic_type, DurationDirective);
  registry.registerDirective(FlowIdDirective.semantic_type, FlowIdDirective);
  registry.registerDirective(
      HashDigestDirective.semantic_type, HashDigestDirective);
  registry.registerDirective(
      HashListDirective.semantic_type, HashListDirective);
  registry.registerDirective(HuntIdDirective.semantic_type, HuntIdDirective);
  registry.registerDirective(JsonDirective.semantic_type, JsonDirective);
  registry.registerDirective(
      MacAddressDirective.semantic_type, MacAddressDirective);
  registry.registerDirective(
      NetworkAddressDirective.semantic_type, NetworkAddressDirective);
  registry.registerDirective(
      ObjectLabelDirective.semantic_type, ObjectLabelDirective);
  registry.registerDirective(
      ObjectLabelDirective.semantic_type, ObjectLabelDirective);
  registry.registerDirective(
      ObjectLabelsListDirective.semantic_type, ObjectLabelsListDirective);
  angular.forEach(PrimitiveDirective.semantic_types, function(type) {
    registry.registerDirective(type, PrimitiveDirective);
  }.bind(this));
  registry.registerDirective(
      SemanticProtoDirective.semantic_type, SemanticProtoDirective);
  registry.registerDirective(
      TimestampDirective.semantic_type, TimestampDirective);
  registry.registerDirective(
      TimestampSecondsDirective.semantic_type, TimestampSecondsDirective);
  registry.registerDirective(UrnDirective.semantic_type, UrnDirective);

  registry.registerDirective(
    VQLDirective.semantic_type, VQLDirective);
});
