'use strict';

goog.module('grrUi.core.errorLabelDirective');

/**
 * Directive that displays a label with the error message whenever a server
 * error occurs
 *
 * @return {!angular.Directive} Directive definition object.
 * @ngInject
 * @export
 */
exports.ErrorLabelDirective = function() {
  return {
      scope: {
          message: '=',
      },
      restrict: 'E',
      template: '<div class="navbar-text error-message" >' +
          '    {$ message $}' +
          '</div>',
  };
};

var ErrorLabelDirective = exports.ErrorLabelDirective;

/**
 * Name of the directive in Angular.
 *
 * @const
 * @export
 */
ErrorLabelDirective.directive_name = 'grrErrorLabel';
