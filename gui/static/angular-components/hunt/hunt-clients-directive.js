'use strict';

goog.module('grrUi.hunt.huntClientsDirective');
goog.module.declareLegacyNamespace();



/**
 * Controller for HuntClientsDirective.
 *
 * @constructor
 * @param {!angular.Scope} $scope
 * @ngInject
 */
const HuntClientsController = function($scope) {
  /** @private {!angular.Scope} */
    this.scope_ = $scope;

    this.params;

    this.scope_.$watchGroup(['huntId'],
                            this.onContextChange_.bind(this));

};


HuntClientsController.prototype.onContextChange_ = function(newValues, oldValues) {
    if (newValues != oldValues || this.pageData == null) {
        this.params = {path: "hunts/" + this.scope_.huntId + ".csv"};
    }
};


/**
 * Directive for displaying clients of a hunt with a given ID.
 *
 * @return {angular.Directive} Directive definition object.
 * @export
 */
exports.HuntClientsDirective = function() {
  return {
    scope: {
      huntId: '='
    },
    restrict: 'E',
    templateUrl: '/static/angular-components/hunt/hunt-clients.html',
    controller: HuntClientsController,
    controllerAs: 'controller'
  };
};


/**
 * Directive's name in Angular.
 *
 * @const
 * @export
 */
exports.HuntClientsDirective.directive_name = 'grrHuntClients';
