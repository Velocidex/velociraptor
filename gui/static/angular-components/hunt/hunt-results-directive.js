'use strict';

goog.module('grrUi.hunt.huntResultsDirective');
goog.module.declareLegacyNamespace();

/**
 * Controller for HuntResultsDirective.
 *
 * @constructor
 * @param {!angular.Scope} $scope
 * @ngInject
 */
const HuntResultsController = function(
    $scope) {
    /** @private {!angular.Scope} */
    this.scope_ = $scope;

    /** @type {array} */
    this.artifactNames;

    /** @type {string} */
    this.selectedArtifact;

    $scope.$watch('hunt.hunt_id', this.onHuntIdChange.bind(this));
    $scope.$watch('controller.selectedArtifact', this.onSelectedArtifactChange.bind(this));
};


HuntResultsController.prototype.onHuntIdChange = function(huntId) {
    if (angular.isObject(this.scope_.hunt)) {
        this.artifactNames = this.scope_.hunt.artifact_sources;

        if (angular.isDefined(this.artifactNames) &&
            this.artifactNames.length > 0) {
            this.selectedArtifact = this.artifactNames[0];
        }
    };
};


HuntResultsController.prototype.onSelectedArtifactChange = function(artifact) {
    if (!angular.isString(artifact)) {
        return;
    }

    this.queryParams = {'hunt_id': this.scope_.huntId,
                        path: this.scope_.huntId,
                        artifact: this.selectedArtifact};
};

/**
 * Directive for displaying results of a hunt with a given ID.
 *
 * @return {!angular.Directive} Directive definition object.
 * @ngInject
 * @export
 */
exports.HuntResultsDirective = function() {
  return {
    scope: {
        hunt: '=',
        huntId: '=',
    },
    restrict: 'E',
    templateUrl: window.base_path+'/static/angular-components/hunt/hunt-results.html',
    controller: HuntResultsController,
    controllerAs: 'controller'
  };
};


/**
 * Directive's name in Angular.
 *
 * @const
 * @export
 */
exports.HuntResultsDirective.directive_name = 'grrHuntResults';
