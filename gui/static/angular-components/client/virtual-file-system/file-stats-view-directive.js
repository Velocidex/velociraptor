'use strict';

goog.module('grrUi.client.virtualFileSystem.fileStatsViewDirective');

const {REFRESH_FILE_EVENT, REFRESH_FOLDER_EVENT} = goog.require('grrUi.client.virtualFileSystem.events');
const {ServerErrorButtonDirective} = goog.require('grrUi.core.serverErrorButtonDirective');
const {SplitPathComponents, Join} = goog.require('grrUi.core.utils');


var OPERATION_POLL_INTERVAL_MS = 1000;

const FileStatsViewController = function(
    $rootScope, $scope, $interval, grrApiService) {
    /** @private {!angular.Scope} */
    this.scope_ = $scope;
    this.rootScope_ = $rootScope;

    this.interval_ = $interval;
    /** @private {!grrUi.core.apiService.ApiService} */
    this.grrApiService_ = grrApiService;

    /** @type {!grrUi.client.virtualFileSystem.fileContextDirective.FileContextController} */
    this.fileContext;

    /** @type {?object} */
    this.updateOperation = {};

    /** @private {!angular.$q.Promise} */
    this.updateOperationInterval_;

    /** @private {boolean} */
    this.updateInProgress;

    this.scope_.$on('$destroy',
                    this.stopMonitorUpdateOperation_.bind(this));

    this.uiTraits = {};
    this.grrApiService_.getCached('v1/GetUserUITraits').then(function(response) {
        this.uiTraits = response.data['interface_traits'];
    }.bind(this), function(error) {
        if (error['status'] == 403) {
            this.error = 'Authentication Error';
        } else {
            this.error = error['statusText'] || ('Error');
        }
    }.bind(this));

};


/**
 * getAccessorAndPath converts a Velociraptor VFS path into a path
 * suitable to use on the client.
 *
 * @export
 */
FileStatsViewController.prototype.getAccessorAndPath = function(path) {
    var components = SplitPathComponents(path);
    var accessor = 'file';
    if (components.length > 0) {
        accessor = components[0];
        path = components.slice(1).join("/");
    }

    if (accessor == 'ntfs' && components.length > 1) {
        path = components[1] + "\\" + components.slice(2).join("\\");
    }

    return {accessor: accessor, path: path};
};

FileStatsViewController.prototype.updateFile = function() {
  var self = this;

  if (this.updateInProgress) {
      self.stopMonitorUpdateOperation_();
  }

  var current_mtime = 0;
  if (angular.isObject(this.fileContext) &&
      angular.isObject(this.fileContext.selectedRow) &&
      angular.isObject(this.fileContext.selectedRow.Download)) {
    current_mtime = this.fileContext.selectedRow.Download.mtime;
  }

  var clientId = this.fileContext['clientId'];
  var selectedFilePath = this.fileContext['selectedFilePath'];
  var components = this.getAccessorAndPath(selectedFilePath);

  var url = 'v1/CollectArtifact';
  var params = {
      client_id: clientId,
      artifacts: ["System.VFS.DownloadFile"],
      parameters: {
          env: [{key: "Path", value: components.path},
                {key: "Accessor", value: components.accessor}],
      }
  };

  this.updateInProgress = true;
  this.grrApiService_.post(url, params).then(
      function success(response) {
        self.updateOperation = {
            mtime:    current_mtime,
            path: components.path,
            accessor: components.accessor,
            vfs_path: selectedFilePath,
        };
        self.monitorUpdateOperation_();
      }.bind(this),
      function failure(response) {
        this.stopMonitorUpdateOperation_();
      }.bind(this));
};

/**
 * Periodically checks the update operation.
 *
 * @private
 */
FileStatsViewController.prototype.monitorUpdateOperation_ = function() {
  this.updateOperationInterval_ = this.interval_(
      this.pollUpdateOperationState_.bind(this),
      OPERATION_POLL_INTERVAL_MS);
};

/**
 * Polls the update operation state.
 *
 * @private
 */
FileStatsViewController.prototype.pollUpdateOperationState_ = function() {
  var clientId = this.fileContext['clientId'];
  var url = 'v1/VFSStatDownload';
  var params = {
      client_id: clientId,
      path: this.updateOperation.path,
      accessor: this.updateOperation.accessor,
      vfs_path: this.updateOperation.vfs_path
  };
  this.grrApiService_.get(url, params).then(
    function success(response) {
      // The mtime changed - stop the polling.
      var mtime = 0;
      if (angular.isDefined(response.data.mtime)) {
        mtime = response.data.mtime;
      }

      if (mtime != this.updateOperation.mtime) {

        this.fileContext.selectedRow.Download = response.data;
        this.stopMonitorUpdateOperation_();

        // Force a refresh on the file table which is watching this
        // parameter.
        this.rootScope_.$broadcast(REFRESH_FOLDER_EVENT);
        this.rootScope_.$broadcast(REFRESH_FILE_EVENT);
      }
    }.bind(this),
    function failure(response) {
      this.stopMonitorUpdateOperation_();
    }.bind(this));
};

/**
 * Stops the peridoc operation state checks.
 *
 * @private
 */
FileStatsViewController.prototype.stopMonitorUpdateOperation_ = function() {
  this.updateOperation = {};
  this.updateInProgress = false;
  this.interval_.cancel(this.updateOperationInterval_);
};

/**
 * Downloades the file.
 *
 * @export
 */
FileStatsViewController.prototype.downloadFile = function() {
  var filePath = this.fileContext.selectedRow.Download.vfs_path;
  var clientId = this.fileContext['clientId'];

  var url = 'v1/DownloadVFSFile';
  var params = {
    vfs_path: filePath,
    client_id: clientId,
  };
  this.grrApiService_.downloadFile(url, params).then(
    function success() {}.bind(this),
  );
};


/**
 * FileStatsViewDirective definition.
 * @return {angular.Directive} Directive definition object.
 */
exports.FileStatsViewDirective = function() {
  return {
    restrict: 'E',
    scope: {},
    require: '^grrFileContext',
    templateUrl: window.base_path+'/static/angular-components/client/virtual-file-system/file-stats-view.html',
    controller: FileStatsViewController,
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
exports.FileStatsViewDirective.directive_name = 'grrFileStatsView';
