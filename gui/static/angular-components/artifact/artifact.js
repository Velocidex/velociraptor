'use strict';

goog.module('grrUi.artifact.artifact');
goog.module.declareLegacyNamespace();

const {ArtifactsViewerDirective} = goog.require('grrUi.artifact.artifactViewerDirective');
const {AceService} = goog.require('grrUi.core.aceService');
const {FormDirective} = goog.require('grrUi.artifact.formDirective');
const {LineChartDirective} = goog.require('grrUi.artifact.lineChartDirective');
const {TimelineDirective} = goog.require('grrUi.artifact.timelineDirective');
const {ReportingDirective} = goog.require('grrUi.artifact.reportingDirective');
const {SearchArtifactDirective} = goog.require('grrUi.artifact.searchArtifactDirective');
const {ClientEventDirective} = goog.require('grrUi.artifact.clientEventDirective');
const {ServerArtifactsDirective} = goog.require('grrUi.artifact.serverArtifactsDirective');
const {ServerEventsDirective} = goog.require('grrUi.artifact.serverEventsDirective');
const {AddArtifactDirective} = goog.require('grrUi.artifact.addArtifactDirective');
const {coreModule} = goog.require('grrUi.core.core');

/**
 * Module with artifact-related directives.
 */
exports.artifactModule = angular.module(
    'grrUi.artifact',
    [coreModule.name, 'ui.ace', 'ui.bootstrap']);

exports.artifactModule.directive(
    ArtifactsViewerDirective.directive_name, ArtifactsViewerDirective);

exports.artifactModule.directive(
    FormDirective.directive_name, FormDirective);

exports.artifactModule.directive(
    LineChartDirective.directive_name, LineChartDirective);
exports.artifactModule.directive(
    TimelineDirective.directive_name, TimelineDirective);

exports.artifactModule.directive(
  ReportingDirective.directive_name, ReportingDirective);

exports.artifactModule.directive(
  SearchArtifactDirective.directive_name,
  SearchArtifactDirective);

exports.artifactModule.directive(
  ClientEventDirective.directive_name,
  ClientEventDirective);

exports.artifactModule.directive(
    ServerArtifactsDirective.directive_name,
    ServerArtifactsDirective);
exports.artifactModule.directive(
    ServerEventsDirective.directive_name,
    ServerEventsDirective);

exports.artifactModule.directive(
    AddArtifactDirective.directive_name,
    AddArtifactDirective);
