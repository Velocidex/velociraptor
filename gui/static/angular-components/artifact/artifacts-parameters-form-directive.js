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
                value: descriptor.parameters[i].default
            });
        };
    }

    // Make sure the artifact is added.
    // this.scope_.$parent.controller.add();
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
