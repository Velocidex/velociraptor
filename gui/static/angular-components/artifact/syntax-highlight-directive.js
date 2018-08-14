'use strict';

goog.module('grrUi.artifact.syntaxHighlightDirective');
goog.module.declareLegacyNamespace();


const SyntaxHighlightDirective = function($scope, $sce) {
  this.sce_ = $sce;
  this.scope_ = $scope;
  this.language = this.scope_.language;

  $scope.$watch('code', this.onCodeChange_.bind(this));
};

SyntaxHighlightDirective.prototype.onCodeChange_ = function(code) {
  if (angular.isDefined(this.language)) {
    var highlighted_code = hljs.highlight(this.language, code);
    this.scope_.rendered = this.sce_.trustAsHtml(highlighted_code.value);
  };
};

exports.SyntaxHighlightDirective = function() {
  return {
    scope: {
      language: '@',
      code: '=',
    },
    restrict: 'E',
    template: '<div class="code" ng-bind-html="rendered"></div>',
    controller: SyntaxHighlightDirective,
    controllerAs: 'controller'
  };
};


exports.SyntaxHighlightDirective.directive_name = 'grrSyntaxHighlight';
