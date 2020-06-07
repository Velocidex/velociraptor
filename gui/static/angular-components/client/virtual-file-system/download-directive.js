'use strict';

goog.module('grrUi.client.virtualFileSystem.downloadDirective');
goog.module.declareLegacyNamespace();

/**
 * Controller for DownloadDirective.
 *
 * @param {!angular.Scope} $scope
 * @param {!angular.jQuery} $element
 * @constructor
 * @ngInject
 */
const DownloadController = function(
    $scope, $element) {
  /** @private {!angular.Scope} */
  this.scope_ = $scope;
};

exports.DownloadDirective = function() {
  return {
    scope: {
      value: '=',
    },
    restrict: 'E',
    templateUrl: '/static/angular-components/client/virtual-file-system/download.html',
    controller: DownloadController,
    controllerAs: 'controller'
  };
};


/**
 * Directive's name in Angular.
 *
 * @const
 * @export
 */
exports.DownloadDirective.directive_name = 'grrVfsDownload';
