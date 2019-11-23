'use strict';

goog.module('grrUi.flow.flow');
goog.module.declareLegacyNamespace();

const {ClientFlowsListDirective} = goog.require('grrUi.flow.clientFlowsListDirective');
const {ClientFlowsViewDirective} = goog.require('grrUi.flow.clientFlowsViewDirective');
const {FlowInspectorDirective} = goog.require('grrUi.flow.flowInspectorDirective');
const {FlowLogDirective} = goog.require('grrUi.flow.flowLogDirective');
const {FlowOverviewDirective} = goog.require('grrUi.flow.flowOverviewDirective');
const {FlowRequestsDirective} = goog.require('grrUi.flow.flowRequestsDirective');
const {FlowResultsDirective} = goog.require('grrUi.flow.flowResultsDirective');
const {FlowStatusIconDirective} = goog.require('grrUi.flow.flowStatusIconDirective');
const {FlowReportDirective} = goog.require('grrUi.flow.flowReportDirective');
const {FlowsListDirective} = goog.require('grrUi.flow.flowsListDirective');
const {NewArtifactCollectionDirective} = goog.require('grrUi.flow.newArtifactCollectionDirective');
const {coreModule} = goog.require('grrUi.core.core');


/**
 * Angular module for flows-related UI.
 */
exports.flowModule = angular.module('grrUi.flow', [coreModule.name]);


exports.flowModule.directive(
    ClientFlowsListDirective.directive_name, ClientFlowsListDirective);
exports.flowModule.directive(
    ClientFlowsViewDirective.directive_name, ClientFlowsViewDirective);
exports.flowModule.directive(
    FlowInspectorDirective.directive_name, FlowInspectorDirective);
exports.flowModule.directive(FlowLogDirective.directive_name, FlowLogDirective);
exports.flowModule.directive(
    FlowOverviewDirective.directive_name, FlowOverviewDirective);
exports.flowModule.directive(
    FlowRequestsDirective.directive_name, FlowRequestsDirective);
exports.flowModule.directive(
    FlowResultsDirective.directive_name, FlowResultsDirective);
exports.flowModule.directive(
    FlowReportDirective.directive_name, FlowReportDirective);
exports.flowModule.directive(
    FlowStatusIconDirective.directive_name, FlowStatusIconDirective);
exports.flowModule.directive(
    FlowsListDirective.directive_name, FlowsListDirective);
exports.flowModule.directive(
    NewArtifactCollectionDirective.directive_name, NewArtifactCollectionDirective);
