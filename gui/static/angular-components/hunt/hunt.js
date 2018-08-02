'use strict';

goog.module('grrUi.hunt.hunt');
goog.module.declareLegacyNamespace();

const {HuntClientsDirective} = goog.require('grrUi.hunt.huntClientsDirective');
const {HuntInspectorDirective} = goog.require('grrUi.hunt.huntInspectorDirective');
const {HuntOverviewDirective} = goog.require('grrUi.hunt.huntOverviewDirective');
const {HuntResultsDirective} = goog.require('grrUi.hunt.huntResultsDirective');
const {HuntStatusIconDirective} = goog.require('grrUi.hunt.huntStatusIconDirective');
const {HuntsListDirective} = goog.require('grrUi.hunt.huntsListDirective');
const {HuntsViewDirective} = goog.require('grrUi.hunt.huntsViewDirective');
const {ModifyHuntDialogDirective} = goog.require('grrUi.hunt.modifyHuntDialogDirective');
const {coreModule} = goog.require('grrUi.core.core');
const {newHuntWizardModule} = goog.require('grrUi.hunt.newHuntWizard.newHuntWizard');


/**
 * Angular module for hunts-related UI.
 */
exports.huntModule = angular.module(
    'grrUi.hunt', ['ui.bootstrap', coreModule.name, newHuntWizardModule.name]);


exports.huntModule.directive(
    HuntClientsDirective.directive_name, HuntClientsDirective);
exports.huntModule.directive(
    HuntInspectorDirective.directive_name, HuntInspectorDirective);
exports.huntModule.directive(
    HuntOverviewDirective.directive_name, HuntOverviewDirective);
exports.huntModule.directive(
    HuntResultsDirective.directive_name, HuntResultsDirective);
exports.huntModule.directive(
    HuntStatusIconDirective.directive_name, HuntStatusIconDirective);
exports.huntModule.directive(
    HuntsListDirective.directive_name, HuntsListDirective);
exports.huntModule.directive(
    HuntsViewDirective.directive_name, HuntsViewDirective);
exports.huntModule.directive(
    ModifyHuntDialogDirective.directive_name, ModifyHuntDialogDirective);
