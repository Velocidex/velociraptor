'use strict';

goog.module('grrUi.core.inspectJsonDirective');

const InspectJsonController = function($scope) {
    this.scope_ = $scope;
    this.scope_.aceConfig = function(ace) {
        self.ace = ace;
        self.ace.resize();
    };
};


exports.InspectJsonDirective = function() {
  return {
      scope: {
          json: "=",
          onResolve: '&',
      },
      restrict: 'E',
      templateUrl: '/static/angular-components/core/inspect-json.html',
      controller: InspectJsonController,
      controllerAs: 'controller'
  };
};

exports.InspectJsonDirective.directive_name = 'grrInspectJson';
