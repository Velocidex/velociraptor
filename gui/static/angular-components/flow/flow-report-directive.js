'use strict';

goog.module('grrUi.flow.flowReportDirective');

/**
 * Controller for FlowReportDirective.
 *
 * @constructor
 * @param {!angular.Scope} $scope
 * @ngInject
 */
const FlowReportController = function($scope) {
  /** @private {!angular.Scope} */
  this.scope_ = $scope;

    /** @type {string} */
    this.flow;
    this.reporting_params;
    this.selectedArtifact;
    this.artifactNames;


    this.scope_.$watchGroup(
        ['flow_id', 'artifactNames', 'controller.selectedArtifact'],
        this.onFlowChange_.bind(this));
};

FlowReportController.prototype.onFlowChange_ = function(
    newValues, oldValues) {

   if (angular.isDefined(this.scope_.artifactNames) &&
        this.scope_.artifactNames.length > 0 &&
        this.selectedArtifact == null) {
        this.selectedArtifact = this.scope_.artifactNames[0];
    }

        var flow_id = this.scope_["flowId"];
        var client_id = this.scope_["clientId"];

        this.reporting_params = {
            "artifact": this.selectedArtifact,
            "client_id": client_id,
            "flowId": flow_id,
            "type": "CLIENT",
        };

        this.artifactNames = this.scope_["artifactNames"];
        if (!angular.isDefined(this.selectedArtifact) &&
            this.artifactNames.length > 0 ) {
            this.selectedArtifact = this.artifactNames[0];
        }
};

/**
 * Directive for displaying results of a flow with a given URL.
 *
 * @return {!angular.Directive} Directive definition object.
 * @ngInject
 * @export
 */
exports.FlowReportDirective = function() {
  return {
    scope: {
        artifactNames: '=',
        flowId: '=',
        clientId: '=',
    },
    restrict: 'E',
    templateUrl: '/static/angular-components/flow/flow-report.html',
    controller: FlowReportController,
    controllerAs: 'controller'
  };
};


/**
 * Directive's name in Angular.
 *
 * @const
 * @export
 */
exports.FlowReportDirective.directive_name = 'grrFlowReport';
