'use strict';

goog.module('grrUi.hunt.huntOverviewDirective');
goog.module.declareLegacyNamespace();

/**
 * Controller for HuntOverviewDirective.
 *
 * @constructor
 * @param {!angular.Scope} $scope
 * @ngInject
 */
const HuntOverviewController = function($scope) {
  /** @private {!angular.Scope} */
  this.scope_ = $scope;

  /** @type {string} */
  this.scope_.hunt;

};


/**
 * Directive for displaying log records of a hunt with a given URN.
 *
 * @return {!angular.Directive} Directive definition object.
 * @ngInject
 * @export
 */
exports.HuntOverviewDirective = function() {
  return {
    scope: {
      hunt: '=',
    },
    restrict: 'E',
    templateUrl: '/static/angular-components/hunt/hunt-overview.html',
    controller: HuntOverviewController,
    controllerAs: 'controller'
  };
};


/**
 * Directive's name in Angular.
 *
 * @const
 * @export
 */
exports.HuntOverviewDirective.directive_name = 'grrHuntOverview';
