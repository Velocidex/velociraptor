'use strict';

goog.module('grrUi.client.virtualFileSystem.fileContextDirective');
goog.module.declareLegacyNamespace();



/**
 * Controller for FileContextDirective.
 *
 * @constructor
 * @param {!angular.Scope} $scope
 * @ngInject
 */
exports.FileContextController = function($scope) {
  /** @private {!angular.Scope} */
  this.scope_ = $scope;

  /** @type {string} */
  this.clientId;

  /** @type {string} */
  this.selectedDirPath;

  this.scope_.$watchGroup(['clientId', 'selectedDirPath'],
      this.onDirectiveArgumentsChange_.bind(this));

    this.scope_.$watchGroup(['controller.clientId', 'controller.selectedDirPath'],
      this.onControllerValuesChange_.bind(this));
};

var FileContextController = exports.FileContextController;


/**
 * Is triggered whenever any value on the scope changes.
 *
 * @private
 */
FileContextController.prototype.onDirectiveArgumentsChange_ = function() {
  this.clientId = this.scope_['clientId'];
  this.selectedDirPath = this.scope_['selectedDirPath'];
};


/**
 * Is triggered whenever any value in the controller changes.
 *
 * @private
 */
FileContextController.prototype.onControllerValuesChange_ = function() {
  this.scope_['clientId'] = this.clientId;
  this.scope_['selectedDirPath'] = this.selectedDirPath;
};


/**
 * Selects a file and updates the scope.
 *
 * @param {string} filePath The path to the selected file within the same folder as the previous seleciton.
 */
FileContextController.prototype.selectFile = function(filePath, opt_fileVersion) {
  this.selectedDirPath = filePath;
  this.selectedRow = {};
};


/**
 * FileContextDirective definition.
 * @return {angular.Directive} Directive definition object.
 */
exports.FileContextDirective = function() {
  return {
    restrict: 'E',
    scope: {
      clientId: '=',
      selectedDirPath: '=',
      selectedRow: '=',
    },
    transclude: true,
    template: '<ng-transclude />',
    controller: FileContextController,
    controllerAs: 'controller'
  };
};


/**
 * Name of the directive in Angular.
 *
 * @const
 * @export
 */
exports.FileContextDirective.directive_name = 'grrFileContext';
