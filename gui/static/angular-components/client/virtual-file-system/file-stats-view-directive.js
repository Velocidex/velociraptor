'use strict';

goog.module('grrUi.client.virtualFileSystem.fileStatsViewDirective');

const {REFRESH_FILE_EVENT, REFRESH_FOLDER_EVENT} = goog.require('grrUi.client.virtualFileSystem.events');
const {ServerErrorButtonDirective} = goog.require('grrUi.core.serverErrorButtonDirective');


var ERROR_EVENT_NAME = ServerErrorButtonDirective.error_event_name;
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

    /** @type {?string} */
    this.updateOperationId;

    /** @private {!angular.$q.Promise} */
    this.updateOperationInterval_;

    /** @private {boolean} */
    this.updateInProgress;

    this.scope_.$on('$destroy',
                    this.stopMonitorUpdateOperation_.bind(this));
};


/**
 * Updates the current file.
 *
 * @export
 */
FileStatsViewController.prototype.updateFile = function() {
  if (this.updateInProgress) {
    return;
  }

  var clientId = this.fileContext['clientId'];
  var selectedFilePath = this.fileContext['selectedFilePath'];
  var url = 'v1/LaunchFlow';
  var params = {
    client_id: clientId,
    flow_name: "VFSDownloadFile",
    args: {
      '@type': 'type.googleapis.com/proto.VFSDownloadFileRequest',
      vfs_path: [selectedFilePath],
    }
  };

  this.updateInProgress = true;
  this.grrApiService_.post(url, params).then(
      function success(response) {
        this.updateOperationId = response['data']['flow_id'];
        this.monitorUpdateOperation_();
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
  var url = 'v1/GetFlowDetails/' + clientId;
  var params = {
    flow_id: this.updateOperationId,
  };
  this.grrApiService_.get(url, params).then(
    function success(response) {
        if (response['data']['context']['state'] != 'RUNNING') {
            this.rootScope_.$broadcast(REFRESH_FOLDER_EVENT);
            this.rootScope_.$broadcast(REFRESH_FILE_EVENT);

            // Force a refresh on the file table which is watching this
            // parameter.
            this.fileContext.flowId = this.updateOperationId;
            this.stopMonitorUpdateOperation_();
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
  this.updateOperationId = null;
  this.updateInProgress = false;
  this.interval_.cancel(this.updateOperationInterval_);
};

/**
 * Downloades the file.
 *
 * @export
 */
FileStatsViewController.prototype.downloadFile = function() {
  var clientId = this.fileContext['clientId'];
  var filePath = this.fileContext['selectedFilePath'];
  var filename = clientId + '/' + filePath;

  // Sanitize filename for download.
  var url = 'v1/DownloadVFSFile/' + filename.replace(/[^a-zA-Z0-9]+/g, '_');
  var params = {
    client_id: clientId,
    vfs_path: filePath,
  };
  this.grrApiService_.downloadFile(url, params).then(
    function success() {}.bind(this),
    function failure(response) {
      if (angular.isUndefined(response.status)) {
        this.rootScope_.$broadcast(
          ERROR_EVENT_NAME, {
            message: 'Couldn\'t download file.'
          });
      }
    }.bind(this)
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
    templateUrl: '/static/angular-components/client/virtual-file-system/file-stats-view.html',
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
