'use strict';

goog.module('grrUi.client.virtualFileSystem.fileDetailsDirective');
goog.module.declareLegacyNamespace();

const {REFRESH_FILE_EVENT} = goog.require('grrUi.client.virtualFileSystem.events');

/**
 * Controller for FileDetailsDirective.
 *
 * @constructor
 * @param {!angular.Scope} $scope
 * @param {!grrUi.core.apiService.ApiService} grrApiService
 * @ngInject
 */
const FileDetailsController = function(
    $scope, grrApiService) {
  /** @private {!angular.Scope} */
  this.scope_ = $scope;

  /** @private {!grrUi.core.apiService.ApiService} */
  this.grrApiService_ = grrApiService;

  /** @type {!grrUi.client.virtualFileSystem.fileContextDirective.FileContextController} */
  this.fileContext;

  /** @type {string} */
  this.currentTab = 'stats';

  this.scope_.$watch('currentTab', this.onDirectiveTabChange_.bind(this));
  this.scope_.$watch('controller.currentTab', this.onControllerTabChange_.bind(this));
};



/**
 * Handles currentTab scope attribute changes.
 *
 * @param {string} newValue
 * @private
 */
FileDetailsController.prototype.onDirectiveTabChange_ = function(newValue) {
  if (angular.isString(newValue)) {
    this.currentTab = newValue;
  }
};

/**
 * Handles controller's currentTab attribute changes.
 *
 * @param {string} newValue
 * @param {string} oldValue
 * @private
 */
FileDetailsController.prototype.onControllerTabChange_ = function(newValue, oldValue) {
  if (newValue !== oldValue) {
    this.scope_['currentTab'] = newValue;
  }
};


FileDetailsController.prototype.fileIsNotCSV = function() {
    return false;
    return !this.fileContext.selectedFilePath.endsWith(".csv");
};



/**
 * FileDetailsDirective definition.
 *
 * @return {angular.Directive} Directive definition object.
 */
exports.FileDetailsDirective = function() {
  return {
    restrict: 'E',
    scope: {
      currentTab: '='
    },
    require: '^grrFileContext',
    templateUrl: '/static/angular-components/client/virtual-file-system/file-details.html',
    controller: FileDetailsController,
    controllerAs: 'controller',
    link: function(scope, element, attrs, fileContextController) {
      scope.controller.fileContext = fileContextController;
    }
  };
};


/**
 * Name of the directive in Angular.
 *
 * @const
 * @export
 */
exports.FileDetailsDirective.directive_name = 'grrFileDetails';
