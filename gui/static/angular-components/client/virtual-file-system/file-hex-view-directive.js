'use strict';

goog.module('grrUi.client.virtualFileSystem.fileHexViewDirective');
goog.module.declareLegacyNamespace();



/**
 * Controller for FileHexViewDirective.
 *
 * @constructor
 * @param {!angular.Scope} $scope
 * @param {!grrUi.core.apiService.ApiService} grrApiService
 * @ngInject
 */
const FileHexViewController = function(
    $scope, grrApiService) {
  /** @private {!angular.Scope} */
  this.scope_ = $scope;

  /** @private {!grrUi.core.apiService.ApiService} */
  this.grrApiService_ = grrApiService;

  /** @type {!grrUi.client.virtualFileSystem.fileContextDirective.FileContextController} */
  this.fileContext;

  /** @type {Array} */
  this.hexDataRows;

  /** @export {number} */
  this.page = 1;

  /** @export {number} */
  this.pageCount = 1;

  /** @private {number} */
  this.rows_ = 25;

  /** @private {number} */
  this.columns_ = 0x14;

  /** @private {number} */
  this.offset_ = 0;

  /** @private {number} */
  this.chunkSize_ = (this.rows_) * this.columns_;

  this.scope_.$watchGroup(['controller.fileContext.clientId',
                           'controller.fileContext.selectedFilePath',
                           'controller.fileContext.selectedFileVersion'],
      this.onContextChange_.bind(this));

  this.scope_.$watch('controller.page', this.onPageChange_.bind(this));
};



/**
 * Handles changes to the clientId and filePath.
 *
 * @private
 */
FileHexViewController.prototype.onContextChange_ = function() {
  var clientId = this.fileContext['clientId'];
  var filePath = this.fileContext['selectedFilePath'];

  if (angular.isDefined(clientId) && angular.isDefined(filePath)) {
    this.fetchText_();
  }
};

/**
 * Handles changes to the selected page.
 * @param {number} page
 * @param {number} oldPage
 * @private
 */
FileHexViewController.prototype.onPageChange_ = function(page, oldPage) {
  if (this.page !== oldPage) {
    this.offset_ = (this.page - 1) * this.chunkSize_;
    this.fetchText_();
  }
};

/**
 * Fetches the file content.
 *
 * @private
 */
FileHexViewController.prototype.fetchText_ = function() {
  var clientId = this.fileContext['clientId'];
  var filePath = this.fileContext['selectedFilePath'];
  var total_size = this.fileContext.selectedRow['Size'];

  this.pageCount = Math.ceil(total_size / this.chunkSize_);

  var url = 'v1/DownloadVFSFile/' + clientId + '/' + filePath;
  var params = {
    offset: this.offset_,
    length: this.chunkSize_,
    vfs_path: filePath,
    client_id: clientId,
  };

  this.grrApiService_.get(url, params).then(function(response) {
    this.parseFileContentToHexRepresentation_(response.data);
  }.bind(this), function() {
    this.hexDataRows = null;
  }.bind(this));
};

/**
 * Parses the string response to a representation better suited for display.
 *
 * @param {string} fileContent The file content as string.
 * @private
 */
FileHexViewController.prototype.parseFileContentToHexRepresentation_ = function(fileContent) {
  this.hexDataRows = [];

  if (!fileContent) {
    return;
  }

  for(var i = 0; i < this.rows_; i++){
    var rowOffset = this.offset_ + (i * this.columns_);
    var data = fileContent.substr(i * this.columns_, this.columns_);
    var data_row = [];
    for (var j = 0; j < data.length; j++) {
      var char = data.charCodeAt(j).toString(16);
      data_row.push(('0' + char).substr(-2)); // add leading zero if necessary
    };

    this.hexDataRows.push({
      offset: rowOffset,
      data_row: data_row,
      data: data,
      safe_data: data.replace(/[^\x20-\x7f]/g, '.'),
    });
  }
};

/**
 * FileHexViewDirective definition.
 *
 * @return {angular.Directive} Directive definition object.
 */
exports.FileHexViewDirective = function() {
  return {
    restrict: 'E',
    scope: {},
    require: '^grrFileContext',
    templateUrl: '/static/angular-components/client/virtual-file-system/file-hex-view.html',
    controller: FileHexViewController,
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
exports.FileHexViewDirective.directive_name = 'grrFileHexView';
