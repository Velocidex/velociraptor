'use strict';

goog.module('grrUi.artifact.artifact');
goog.module.declareLegacyNamespace();

const {ArtifactCollectorDirective} = goog.require('grrUi.artifact.collectorDirective');
const {ArtifactDescriptorDirective} = goog.require('grrUi.artifact.artifactDescriptorDirective');
const {ArtifactDescriptorsService} = goog.require('grrUi.artifact.artifactDescriptorsService');
const {ArtifactsListFormDirective} = goog.require('grrUi.artifact.artifactsListFormDirective');
const {ArtifactsParamsFormDirective} = goog.require('grrUi.artifact.artifactsParamsFormDirective');
const {SyntaxHighlightDirective} = goog.require('grrUi.artifact.syntaxHighlightDirective');
const {coreModule} = goog.require('grrUi.core.core');
const {formsModule} = goog.require('grrUi.forms.forms');
const {semanticModule} = goog.require('grrUi.semantic.semantic');

/**
 * Module with artifact-related directives.
 */
exports.artifactModule = angular.module(
    'grrUi.artifact',
    [coreModule.name, formsModule.name, semanticModule.name, 'ui.bootstrap']);

exports.artifactModule.directive(
    ArtifactCollectorDirective.directive_name, ArtifactCollectorDirective);
exports.artifactModule.directive(
    ArtifactDescriptorDirective.directive_name, ArtifactDescriptorDirective);
exports.artifactModule.directive(
    ArtifactsListFormDirective.directive_name, ArtifactsListFormDirective);
exports.artifactModule.directive(
    ArtifactsParamsFormDirective.directive_name, ArtifactsParamsFormDirective);

exports.artifactModule.directive(
  SyntaxHighlightDirective.directive_name,
  SyntaxHighlightDirective);

exports.artifactModule.service(
    ArtifactDescriptorsService.service_name, ArtifactDescriptorsService);

exports.artifactModule.run(function(
    grrSemanticFormDirectivesRegistryService) {
  var registry = grrSemanticFormDirectivesRegistryService;

    registry.registerDirective(
        ArtifactsListFormDirective.semantic_type, ArtifactsListFormDirective);

    registry.registerDirective(
        ArtifactsParamsFormDirective.semantic_type, ArtifactsParamsFormDirective);
});


exports.artifactModule.run(function(grrSemanticValueDirectivesRegistryService) {
  var registry = grrSemanticValueDirectivesRegistryService;

  registry.registerDirective(
      ArtifactDescriptorDirective.semantic_type, ArtifactDescriptorDirective);
});
