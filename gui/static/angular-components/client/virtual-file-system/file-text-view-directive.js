'use strict';

goog.module('grrUi.client.virtualFileSystem.fileTextViewDirective');
goog.module.declareLegacyNamespace();



/**
 * Controller for FileTextViewDirective.
 *
 * @constructor
 * @param {!angular.Scope} $scope
 * @param {!grrUi.core.apiService.ApiService} grrApiService
 * @ngInject
 */
const FileTextViewController = function(
    $scope, grrApiService) {
  /** @private {!angular.Scope} */
  this.scope_ = $scope;

  /** @private {!grrUi.core.apiService.ApiService} */
  this.grrApiService_ = grrApiService;

  /** @type {!grrUi.client.virtualFileSystem.fileContextDirective.FileContextController} */
  this.fileContext;

  /** @type {?string} */
  this.fileContent;

  /** @export {number} */
  this.page = 1;

  /** @export {number} */
  this.pageCount = 10;

  /** @private {number} */
  this.chunkSize_ = 10000;

  this.language;

  this.scope_.$watchGroup(['controller.fileContext.clientId',
                           'controller.fileContext.selectedFilePath'],
      this.onContextChange_.bind(this));

  this.scope_.$watch('controller.page', this.onPageChange_.bind(this));
};



/**
 * Handles changes to the clientId and filePath.
 *
 * @private
 */
FileTextViewController.prototype.onContextChange_ = function() {
  var clientId = this.fileContext['clientId'];
  var filePath = this.fileContext['selectedFilePath'];

  if (angular.isDefined(clientId) && angular.isDefined(filePath)) {
    this.fetchText_();
  }
};

/**
 * Handles changes to the encoding.
 * @param {number} page
 * @param {number} oldPage
 * @private
 */
FileTextViewController.prototype.onPageChange_ = function(page, oldPage) {
  if (this.page !== oldPage){
    this.fetchText_();
  }
};

FileTextViewController.prototype.guessHighlighting = function(filename) {
  if (filename.match(/yaml$/i)) {
    return "yaml";
  }

  if (filename.match(/csv$/i)) {
    return "yaml";
  }

  return "";
};

/**
 * Fetches the file content.
 *
 * @private
 */
FileTextViewController.prototype.fetchText_ = function() {
  this.fileContent = "Loading....";
  if (angular.isUndefined(this.fileContext.selectedRow)) {
    return;
  }

  var clientId = this.fileContext['clientId'];
  var download = this.fileContext.selectedRow.Download;
  if (download == null) {
    return;
  }

  var filePath = download.vfs_path;

  this.language = this.guessHighlighting(filePath);

  var total_size = this.fileContext.selectedRow['Size'];
  var offset = (this.page - 1) * this.chunkSize_;

  var url = 'v1/DownloadVFSFile/' + clientId + '/' + filePath;
  var params = {
    offset: offset,
    length: this.chunkSize_,
    vfs_path: filePath,
    client_id: clientId,
  };

  this.grrApiService_.get(url, params).then(function(response) {
    this.pageCount = Math.ceil(total_size / this.chunkSize_);
    this.fileContent = response.data;
  }.bind(this), function() {
    this.fileContent = null;
  }.bind(this));
};

/**
 * FileTextViewDirective definition.
 * @return {angular.Directive} Directive definition object.
 */
exports.FileTextViewDirective = function() {
  return {
    restrict: 'E',
    scope: {},
    require: '^grrFileContext',
    templateUrl: '/static/angular-components/client/virtual-file-system/file-text-view.html',
    controller: FileTextViewController,
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
exports.FileTextViewDirective.directive_name = 'grrFileTextView';
