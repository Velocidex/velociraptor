'use strict';

goog.module('grrUi.core.inspectJsonDirective');

const InspectJsonController = function($scope, grrAceService) {
    var self = this;

    this.scope_ = $scope;
    this.scope_.aceConfig = function(ace) {
        grrAceService.AceConfig(ace);

        ace.setOptions({
            autoScrollEditorIntoView: false,
        });

        self.scope_.$on('$destroy', function() {
            grrAceService.SaveAceConfig(ace);
        });

        ace.resize();
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
