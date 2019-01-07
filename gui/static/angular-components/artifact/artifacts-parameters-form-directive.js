'use strict';

goog.module('grrUi.artifact.artifactsParamsFormDirective');
goog.module.declareLegacyNamespace();



const ArtifactsParamsFormController = function($scope, $rootScope) {
    this.scope_ = $scope;
    this.rootScope_ = $rootScope;
};

ArtifactsParamsFormController.prototype.addItem = function() {
    var descriptor = this.rootScope_["selectedArtifact"];
    if (angular.isUndefined(this.scope_.value.env)) {
        this.scope_.value.env = []
    }

    if (angular.isDefined(descriptor)) {
        for (var i=0; i<descriptor.parameters.length; i++) {
            this.scope_.value.env.push({
                key: descriptor.parameters[i].name,
                friendly_name: descriptor.parameters[i].friendly_name,
                value: descriptor.parameters[i].default,
                descriptor: descriptor.parameters[i],
                type: descriptor.parameters[i].type || "string",
            });
        };
    }

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
      value: '='
    },
    templateUrl: '/static/angular-components/artifact/' +
        'artifacts-params-form.html',
    controller: ArtifactsParamsFormController,
    controllerAs: 'controller'
  };
};


exports.ArtifactsParamsFormDirective.directive_name = 'grrArtifactsParamsForm';
exports.ArtifactsParamsFormDirective.semantic_type = 'ArtifactParameters';
