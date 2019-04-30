'use strict';

goog.module('grrUi.artifact.artifactsParamsFormDirective');
goog.module.declareLegacyNamespace();


/**
 * Controller for ArtifactsParamsFormController.
 *
 * @param {!angular.Scope} $scope
 * @param {!angular.Scope} $rootScope
 * @constructor
 * @ngInject
 */
const ArtifactsParamsFormController = function($scope, $rootScope) {
    /** @private {!angular.Scope} */
    this.scope_ = $scope;
};

ArtifactsParamsFormController.prototype.addItem = function() {
    var parameters = this.scope_["params"];
    var descriptors = this.scope_["descriptors"];

    for (var i=0; i<descriptors.length; i++) {
        parameters.push({
            key: parameters[i].name,
            friendly_name: parameters[i].friendly_name,
            value: parameters[i].default,
            descriptor: parameters[i],
            type: parameters[i].type || "string",
        });
    };

    // Make sure the artifact is added once we start filling in its
    // parameters. This is a bit of a hack but its too hard to figure
    // out how to do it the "angular" way so we just do it the obvious
    // way.
    $("#add_artifact").click();
};


exports.ArtifactsParamsFormDirective = function() {
  return {
    restrict: 'E',
      scope: {
          descriptors: "=",
          params: '='
      },
      templateUrl: '/static/angular-components/artifact/' +
          'artifacts-params-form.html',
      controller: ArtifactsParamsFormController,
      controllerAs: 'controller'
  };
};


exports.ArtifactsParamsFormDirective.directive_name = 'grrArtifactsParamsForm';
