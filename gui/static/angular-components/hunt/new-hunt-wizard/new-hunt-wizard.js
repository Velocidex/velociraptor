'use strict';

goog.module('grrUi.hunt.newHuntWizard.newHuntWizard');
goog.module.declareLegacyNamespace();

const {ConfigureHuntPageDirective} = goog.require('grrUi.hunt.newHuntWizard.configureHuntPageDirective');
const {ConfigureRulesPageDirective} = goog.require('grrUi.hunt.newHuntWizard.configureRulesPageDirective');
const {FormDirective} = goog.require('grrUi.hunt.newHuntWizard.formDirective');
const {StatusPageDirective} = goog.require('grrUi.hunt.newHuntWizard.statusPageDirective');
const {coreModule} = goog.require('grrUi.core.core');


/**
 * Angular module for new hunt wizard UI.
 */
exports.newHuntWizardModule = angular.module(
    'grrUi.hunt.newHuntWizard', ['ui.bootstrap', coreModule.name]);

exports.newHuntWizardModule.directive(
    ConfigureHuntPageDirective.directive_name, ConfigureHuntPageDirective);
exports.newHuntWizardModule.directive(
    ConfigureRulesPageDirective.directive_name, ConfigureRulesPageDirective);
exports.newHuntWizardModule.directive(
    StatusPageDirective.directive_name, StatusPageDirective);
exports.newHuntWizardModule.directive(
    FormDirective.directive_name, FormDirective);
