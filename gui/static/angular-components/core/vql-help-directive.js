'use strict';

goog.module('grrUi.core.vqlHelpDirective');

const VqlHelpController = function($scope, grrAceService) {
    this.scope_ = $scope;
    this.grrAceService_ = grrAceService;
    var self = this;

    this.scope_.aceConfig = function(ace) {
        self.grrAceService_.AceConfig(ace);

        self.scope_.$on('$destroy', function() {
            self.grrAceService_.SaveAceConfig(ace);
        });

        ace.resize();
    };

};


VqlHelpController.prototype.copy = function(e) {
    var copyTextarea = document.querySelector('#clipboard-content');
    copyTextarea.value = this.scope_["vql"];
    copyTextarea.select();
    document.execCommand('copy');

    var onResolve = this.scope_['onResolve'];
    if (onResolve) {
        onResolve();
    }
};

exports.VqlHelpDirective = function() {
  return {
      scope: {
          vql: "=",
          onResolve: '&',
      },
      restrict: 'E',
      templateUrl: '/static/angular-components/core/vql-help.html',
      controller: VqlHelpController,
      controllerAs: 'controller'
  };
};

exports.VqlHelpDirective.directive_name = 'grrVqlHelp';
