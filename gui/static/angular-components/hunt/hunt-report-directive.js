'use strict';

goog.module('grrUi.hunt.huntReportDirective');

const HuntReportController = function($scope) {
  /** @private {!angular.Scope} */
  this.scope_ = $scope;

  /** @type {string} */
  this.hunt;
  this.reporting_params;
  this.artifactNames;

  this.scope_.$watchGroup(
    ['hunt_id'],
    this.onHuntChange_.bind(this));
};

HuntReportController.prototype.onHuntChange_ = function(
    newValues, oldValues) {

   if (angular.isDefined(this.scope_.artifactNames) &&
        this.scope_.artifactNames.length > 0 &&
        this.selectedArtifact == null) {
        this.selectedArtifact = this.scope_.artifactNames[0];
    }

    this.reporting_params = {
      "artifact": this.selectedArtifact,
      "hunt_id":  this.scope_["huntId"],
      "type": "HUNT",
    };

    this.artifactNames = this.scope_["artifactNames"];
    if (!angular.isDefined(this.selectedArtifact) &&
        angular.isDefined(this.artifactNames) &&
        this.artifactNames.length > 0 ) {
        this.selectedArtifact = this.artifactNames[0];
    }
};

exports.HuntReportDirective = function() {
  return {
    scope: {
      hunt: '=',
      huntId: '=',
    },
    restrict: 'E',
    templateUrl: '/static/angular-components/hunt/hunt-report.html',
    controller: HuntReportController,
    controllerAs: 'controller'
  };
};


/**
 * Directive's name in Angular.
 *
 * @const
 * @export
 */
exports.HuntReportDirective.directive_name = 'grrHuntReport';
