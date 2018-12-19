'use strict';

goog.module('grrUi.flow.flowResultsDirective');
goog.module.declareLegacyNamespace();



/**
 * Controller for FlowResultsDirective.
 *
 * @constructor
 * @param {!angular.Scope} $scope
 * @ngInject
 */
const FlowResultsController = function($scope) {
  /** @private {!angular.Scope} */
  this.scope_ = $scope;

  /** @type {?string} */
  this.flowResultsUrl;

  /** @type {?string} */
  this.flowExportedResultsUrl;

  this.exportBasePath;

  /** @type {?string} */
  this.outputPluginsMetadataUrl;

  /** @type {?string} */
  this.downloadFilesUrl;

  /** @type {?string} */
    this.exportCommand;

    this.selectedArtifact;

    this.scope_.$watchGroup(['artifactNames', 'flowId', 'apiBasePath', 'exportBasePath'],
                          this.onFlowIdOrBasePathChange_.bind(this));
};


/**
 * Handles directive's arguments changes.
 *
 * @param {Array<string>} newValues
 * @private
 */
FlowResultsController.prototype.onFlowIdOrBasePathChange_ = function(
    newValues, oldValues) {

    if (newValues != oldValues) {
        this.flowResultsUrl = this.outputPluginsMetadataUrl =
            this.downloadFilesUrl = null;

        if (newValues.every(angular.isDefined)) {
            this.exportedResultsUrl = this.scope_['exportBasePath'] + '/' + this.scope_['flowId'];

            var flowUrl = this.scope_['apiBasePath'] + '/' + this.scope_['flowId'];
            this.flowResultsUrl = flowUrl + '/results';

            if (angular.isDefined(this.scope_.artifactNames) &&
                this.scope_.artifactNames.length > 0 &&
                this.selectedArtifact == null) {
                this.selectedArtifact = this.scope_.artifactNames[0];
            }
        }
    }
};

FlowResultsController.prototype.getPath = function() {
    if (angular.isDefined(this.selectedArtifact) &&
        this.selectedArtifact != null) {
        return '/artifacts/Artifact ' + this.selectedArtifact +
            '/' + this.scope_.flowId + '.csv';
    }
    return '';
}


/**
 * Directive for displaying results of a flow with a given URL.
 *
 * @return {!angular.Directive} Directive definition object.
 * @ngInject
 * @export
 */
exports.FlowResultsDirective = function() {
  return {
    scope: {
        artifactNames: '=',
        flowId: '=',
        clientId: '=',
        apiBasePath: '=',
        exportBasePath: '=',
    },
    restrict: 'E',
    templateUrl: '/static/angular-components/flow/flow-results.html',
    controller: FlowResultsController,
    controllerAs: 'controller'
  };
};


/**
 * Directive's name in Angular.
 *
 * @const
 * @export
 */
exports.FlowResultsDirective.directive_name = 'grrFlowResults';
