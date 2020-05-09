'use strict';

goog.module('grrUi.hunt.newHuntWizard.configureRulesPageDirective');
goog.module.declareLegacyNamespace();


const ConfigureRulesPageController = function(
    $scope, grrRoutingService) {
    this.scope_ = $scope;
    this.scope_["clientRuleSet"]["os"] = "WINDOWS";
};


/**
 * Directive for showing wizard-like forms with multiple named steps/pages.
 *
 * @return {!angular.Directive} Directive definition object.
 * @ngInject
 * @export
 */
exports.ConfigureRulesPageDirective = function() {
    return {
        scope: {
            clientRuleSet: '=',
        },
        restrict: 'E',
        templateUrl: '/static/angular-components/hunt/new-hunt-wizard/' +
            'configure-rules-page.html',
        controller: ConfigureRulesPageController,
        controllerAs: 'controller',
    };
};


/**
 * Directive's name in Angular.
 *
 * @const
 * @export
 */
exports.ConfigureRulesPageDirective.directive_name = 'grrConfigureRulesPage';
