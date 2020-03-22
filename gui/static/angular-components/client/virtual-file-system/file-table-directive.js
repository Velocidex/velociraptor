'use strict';

goog.module('grrUi.client.virtualFileSystem.fileTableDirective');
goog.module.declareLegacyNamespace();

const {getFolderFromPath} = goog.require('grrUi.client.virtualFileSystem.utils');
const {REFRESH_FOLDER_EVENT} = goog.require('grrUi.client.virtualFileSystem.events');
const {PathJoin} = goog.require('grrUi.core.utils');

var OPERATION_POLL_INTERVAL_MS = 1000;

/**
 * Controller for FileTableDirective.
 *
 * @constructor
 * @param {!angular.Scope} $rootScope
 * @param {!angular.Scope} $scope
 * @param {!angular.$interval} $interval
 * @param {!grrUi.core.apiService.ApiService} grrApiService
 * @ngInject
 */
const FileTableController = function(
    $rootScope, $scope, $interval, $uibModal, grrApiService) {
    /** @private {!angular.Scope} */
    this.rootScope_ = $rootScope;

    /** @private {!angular.Scope} */
    this.scope_ = $scope;

    this.uibModal_ = $uibModal;

    /** @private {!angular.$interval} */
    this.interval_ = $interval;

    /** @private {!grrUi.core.apiService.ApiService} */
    this.grrApiService_ = grrApiService;

    /** @private {string} */
    this.selectedDirPath_;

    /** @private {string} */
    this.selecteFilePath_;

    /** @type {?string} */
    this.lastRefreshOperationId;

    /** @type {!grrUi.client.virtualFileSystem.fileContextDirective.FileContextController} */
    this.fileContext;

    this.selectedRow = {};

    // Set the mode for the tool bar:
    // - all: show all buttons.
    // - vfs_files: browsing the vfs filesystem on the client.
    // - server: Showing server files.
    // - artifacts: Showing artifacts.
    this.mode = "all";

    this.scope_.$watch('controller.fileContext.selectedRow',
                       this.onSelectedRowChange_.bind(this));

    this.scope_.$watch('controller.fileContext.selectedDirPath',
                       this.onDirPathChange_.bind(this));

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

FileTableController.prototype.setMode_ = function() {
    if (!angular.isDefined(this.fileContext.selectedDirPath)) {
        return;
    }

    var path = this.fileContext.selectedDirPath;
    // Viewing the server files VFS
    if (this.fileContext.clientId == "") {
        this.mode = "server";

        if (path.startsWith("server_artifacts")) {
            this.mode = "server_artifacts";

        } else if (path.startsWith("artifact_definitions")) {
            this.mode = "artifact_definitions";
        }

        // Viewing the client's VFS
    } else {
        this.mode = "vfs_files";
        if (path.startsWith("artifacts")) {
            this.mode = "artifact_view";

        } else if (path.startsWith("monitoring")) {
            this.mode = "client_monitoring_view";
        }
    }
};

FileTableController.prototype.showHelp = function() {
    var self = this;
    self.modalInstance = self.uibModal_.open({
        templateUrl: '/static/angular-components/client/virtual-file-system/help.html',
        scope: self.scope_,
        size: "lg",
    });
};

/**
 * @param {?string} newValue
 * @param {?string} oldValue
 *
 * @private
 */
FileTableController.prototype.onDirPathChange_ = function(newValue, oldValue) {
    var newFolder = getFolderFromPath(newValue);
    var oldFolder = getFolderFromPath(oldValue);

    if (newFolder !== oldFolder) {
        // Reset the "refresh directory" button state.
        this.lastRefreshOperationId = null;
    }
    this.setMode_();
};


FileTableController.prototype.onSelectedRowChange_ = function(newValue, oldValue) {
  if (angular.isDefined(newValue) && angular.isDefined(newValue.Name)) {
    if (angular.isDefined(this.fileContext.selectedDirPath)) {
        this.fileContext.selectedFilePath = PathJoin(
            this.fileContext.selectedDirPath, newValue.Name);
    }
  }
};


/**
 * Refreshes the current directory.
 *
 * @export
 */
FileTableController.prototype.startVfsRefreshOperation = function() {
    if (this.lastRefreshOperationId) {
        return;
    }

    var clientId = this.fileContext['clientId'];
    var selectedDirPath = this.fileContext['selectedDirPath'];

    var url = 'v1/VFSRefreshDirectory/' + clientId;
    var params = {
        vfs_path: selectedDirPath,
        depth: 0,
    };

  // Setting this.lastRefreshOperationId means that the update button
  // will get disabled immediately.
  this.lastRefreshOperationId = 'unknown';
  this.grrApiService_.post(url, params)
    .then(
      function success(response) {
        this.lastRefreshOperationId = response.data['flow_id'];
        var pollPromise = this.grrApiService_.poll(
          'v1/VFSStatDirectory',
          OPERATION_POLL_INTERVAL_MS, {
            vfs_path: selectedDirPath,
            client_id: clientId,
            flow_id: this.lastRefreshOperationId,
          }, function(response) {
            if (response.data.flow_id == this.lastRefreshOperationId) {
              this.lastRefreshOperationId = undefined;
              return true;
            };
            return false;
          }.bind(this));
        this.scope_.$on('$destroy', function() {
          this.grrApiService_.cancelPoll(pollPromise);
        }.bind(this));

        return pollPromise;
      }.bind(this))
    .then(
      function success() {
        this.rootScope_.$broadcast(
          REFRESH_FOLDER_EVENT, selectedDirPath);
      }.bind(this));
};

/**
 * FileTableDirective definition.
 * @return {angular.Directive} Directive definition object.
 */
exports.FileTableDirective = function() {
  return {
    restrict: 'E',
    scope: {},
    require: '^grrFileContext',
    templateUrl: '/static/angular-components/client/virtual-file-system/file-table.html',
    controller: FileTableController,
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
exports.FileTableDirective.directive_name = 'grrFileTable';
