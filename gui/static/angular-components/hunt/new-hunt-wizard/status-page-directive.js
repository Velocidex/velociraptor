'use strict';

goog.module('grrUi.hunt.newHuntWizard.statusPageDirective');
goog.module.declareLegacyNamespace();



/**
 * Directive for showing wizard-like forms with multiple named steps/pages.
 *
 * @return {!angular.Directive} Directive definition object.
 * @ngInject
 * @export
 */
exports.StatusPageDirective = function() {
  return {
    scope: {
      response: '=',
      createHuntArgs: '=',
    },
    restrict: 'E',
    templateUrl: window.base_path+'/static/angular-components/hunt/new-hunt-wizard/' +
        'status-page.html'
  };
};


/**
 * Directive's name in Angular.
 *
 * @const
 * @export
 */
exports.StatusPageDirective.directive_name = 'grrNewHuntStatusPage';
