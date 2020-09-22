'use strict';

goog.module('grrUi.hunt.huntReportDirective');

const HuntReportController = function($scope) {
  /** @private {!angular.Scope} */
  this.scope_ = $scope;

  /** @type {string} */
  this.reportParams;

  this.scope_.$watchGroup(
    ['hunt.hunt_id'],
    this.onHuntChange_.bind(this));

  this.onHuntChange_();
};

HuntReportController.prototype.onHuntChange_ = function() {
  var hunt = this.scope_["hunt"];
  if (!angular.isDefined(hunt) || hunt.artifacts.length == 0) {
    return;
  }

  this.selectedArtifact = hunt.artifacts[0];
  this.reportParams = {
    "artifact": this.selectedArtifact,
    "hunt_id":  hunt.hunt_id,
    "type": "HUNT",
  };
};

exports.HuntReportDirective = function() {
  return {
    scope: {
      hunt: '=',
      huntId: '=',
    },
    restrict: 'E',
    templateUrl: window.base_path+'/static/angular-components/hunt/hunt-report.html',
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
