'use strict';

goog.module('grrUi.artifact.syntaxHighlightDirective');
goog.module.declareLegacyNamespace();


/**
 * Controller for SyntaxHighlightDirective.
 *
 * @param {!angular.Scope} $scope
 * @param {!grrUi.core.timeService.TimeService} grrTimeService
 * @constructor
 * @ngInject
 */
const SyntaxHighlightDirective = function($scope, $sce) {
    /** @private {?object} */
    this.sce_ = $sce;

    /** @private {!angular.Scope} */
    this.scope_ = $scope;

    /** @private {?string} */
    this.language = this.scope_.language;

    $scope.$watch('code', this.onCodeChange_.bind(this));
};

SyntaxHighlightDirective.prototype.onCodeChange_ = function(code) {
    if (angular.isDefined(this.language) && angular.isDefined(code)) {
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
