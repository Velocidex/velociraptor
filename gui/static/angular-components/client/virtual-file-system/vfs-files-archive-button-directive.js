'use strict';

goog.module('grrUi.client.virtualFileSystem.vfsFilesArchiveButtonDirective');
goog.module.declareLegacyNamespace();

const {ServerErrorButtonDirective} = goog.require('grrUi.core.serverErrorButtonDirective');
const {getFolderFromPath} = goog.require('grrUi.client.virtualFileSystem.utils');

/**
 * Controller for VfsFilesArchiveButtonDirective.
 *
 * @constructor
 * @param {!angular.Scope} $rootScope
 * @param {!angular.Scope} $scope
 * @param {!angular.$timeout} $timeout
 * @param {!grrUi.core.apiService.ApiService} grrApiService
 * @ngInject
 */
const VfsFilesArchiveButtonController = function(
        $rootScope, $scope, $timeout, grrApiService) {
  /** @private {!angular.Scope} */
  this.rootScope_ = $rootScope;

  /** @private {!angular.Scope} */
  this.scope_ = $scope;

  /** @private {!angular.$timeout} */
  this.timeout_ = $timeout;

  /** @private {!grrUi.core.apiService.ApiService} */
  this.grrApiService_ = grrApiService;
};

/**
 * Handles mouse clicks on 'download current folder' dropdown menu item.
 *
 * @param {Event} e
 * @export
 */
VfsFilesArchiveButtonController.prototype.downloadCurrentFolder = function(e) {
    e.preventDefault();

    var folderPath = getFolderFromPath(this.scope_['filePath']);
    var clientId = this.scope_['clientId'];
    var url = 'v1/DownloadVFSFolder';
    var params = {
        client_id: this.scope_['clientId'],
        vfs_path: folderPath,
    };
    this.grrApiService_.downloadFile(url, params).then(
        function success() {}.bind(this),
    );
};

/**
 * VfsFilesArchiveButtonDirective renders a button that shows a dialog that allows
 * users to change their personal settings.
 *
 * @return {!angular.Directive} Directive definition object.
 */
exports.VfsFilesArchiveButtonDirective = function() {
  return {
    scope: {
      clientId: '=',
      filePath: '='
    },
    restrict: 'E',
    templateUrl: '/static/angular-components/client/virtual-file-system/' +
        'vfs-files-archive-button.html',
    controller: VfsFilesArchiveButtonController,
    controllerAs: 'controller'
  };
};


/**
 * Name of the directive in Angular.
 *
 * @const
 * @export
 */
exports.VfsFilesArchiveButtonDirective.directive_name =
    'grrVfsFilesArchiveButton';
