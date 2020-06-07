'use strict';

goog.module('grrUi.core.inspectJsonDirective');

const InspectJsonController = function($scope, grrAceService) {
    var self = this;

    this.scope_ = $scope;
    this.scope_.aceConfig = function(ace) {
        self.ace = ace;
        grrAceService.AceConfig(ace);

        ace.setOptions({
            autoScrollEditorIntoView: false,
            maxLines: null,
        });

        self.scope_.$on('$destroy', function() {
            grrAceService.SaveAceConfig(ace);
        });

        ace.resize();
    };
};

InspectJsonController.prototype.showSettings = function() {
    this.ace.execCommand("showSettingsMenu");
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
