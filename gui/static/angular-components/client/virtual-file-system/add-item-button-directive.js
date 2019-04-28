'use strict';

goog.module('grrUi.client.virtualFileSystem.addItemButtonDirective');
goog.module.declareLegacyNamespace();


/**
 * Controller for AddItemButtonController
 *
 * @param {!angular.Scope} $rootScope
 * @param {!angular.Scope} $scope
 * @param {!grrUi.core.apiService.ApiService} grrApiService
 * @param {!grrUi.core.reflectionService.ReflectionService} grrReflectionService
 * @constructor
 * @ngInject
 */
const AddItemButtonController = function(
    $rootScope, $scope, $timeout, $uibModal, grrApiService, grrReflectionService) {
  /** @private {!angular.Scope} */
  this.rootScope_ = $rootScope;

  /** @private {!angular.Scope} */
  this.scope_ = $scope;

  /** @private {!angular.$timeout} */
  this.timeout_ = $timeout;

  /** @private {!angularUi.$uibModal} */
  this.uibModal_ = $uibModal;

  /** @private {!grrUi.core.apiService.ApiService} */
  this.grrApiService_ = grrApiService;

  /** @private {!grrUi.core.reflectionService.ReflectionService} */
  this.grrReflectionService_ = grrReflectionService;

  /** @type {?string} */
  this.lastOperationId;

  /** @type {Object} */
  this.refreshOperation;

  /** @type {?boolean} */
  this.done;

  /** @type {?string} */
  this.error;

  /** @private {angularUi.$uibModalInstance} */
    this.modalInstance;

    /** @type {?object} */
    this.fileContext;

    this.scope_.$watchGroup(['clientId', 'filePath', 'dirPath'],
                            this.onClientOrPathChange_.bind(this));

    /** @type {?string} */
    this.path;

    /** @type {?string} */
    this.value;

    this.isActive = false;

    this.mode = "add_artifact";

    // Used to add server monitoring artifacts.
    this.flowArguments = {};
};

AddItemButtonController.prototype.onClientOrPathChange_ = function() {
    var self =this;

    var is_valid_path = function(path) {
        if (angular.isUndefined(path)) {
            return false;
        };

        path = path.replace(/\/+/g, "/");

        // Manipulate artifact definitions.
        if (path.startsWith("/artifact_definitions/")) {
            path = path.replace(/builtin/, "custom");

            self.mode = "add_artifact";
            self.isActive = true;
            return path;
        } else if (path.startsWith("/server_artifacts/")) {
            self.mode = "add_server_monitoring";
            self.isActive = true;
            return path;

        } else {
            return false;
        }
    };

    this.isActive = false;
    this.path = is_valid_path(this.scope_.filePath) ||
        is_valid_path(this.scope_.dirPath);
};


AddItemButtonController.prototype.onClick = function() {
    var self =this;

    if (self.mode == "add_artifact") {
        var url = 'v1/GetArtifactFile';
        var params = {
            vfs_path: this.scope_.filePath,
        };
        this.error = "";
        this.grrApiService_.get(url, params).then(function(response) {
            self.value = response['data']['artifact'];
            self.modalInstance = self.uibModal_.open({
                templateUrl: '/static/angular-components/artifact/add_artifact.html',
                scope: self.scope_,
                size: "lg",
            });
        }.bind(this), function() {
        }.bind(this));

    } else if (self.mode == "add_server_monitoring") {
        var url = 'v1/GetServerMonitoringState';
        this.error = "";
        this.grrApiService_.get(url).then(function(response) {
            self.flowArguments = response['data'];
            self.modalInstance = self.uibModal_.open({
                templateUrl: '/static/angular-components/artifact/add_server_monitoring.html',
                scope: self.scope_,
                size: "lg",
            });
        });
    }
};

AddItemButtonController.prototype.saveServerArtifacts = function() {
    var self = this;
    var url = 'v1/SetServerMonitoringState';
    this.grrApiService_.post(
        url, self.flowArguments).then(function(response) {
        if (response.data.error) {
            this.error = response.data['error_message'];
        } else {
            this.modalInstance.close();
        }
    }.bind(this), function(error) {
        this.error = error;
    }.bind(this));
};

AddItemButtonController.prototype.saveArtifact = function() {
    var url = 'v1/SetArtifactFile';
    var params = {
        vfs_path: this.path,
        artifact: this.value,
    };
    this.grrApiService_.post(url, params).then(function(response) {
        if (response.data.error) {
            this.error = response.data['error_message'];
        } else {
            this.modalInstance.close();
        }
    }.bind(this), function(error) {
        this.error = error;
    }.bind(this));
};


exports.AddItemButtonDirective = function() {
  return {
    scope: {
        clientId: '=',
        filePath: '=',
        dirPath: '=',
    },
    require: '^grrFileContext',
    restrict: 'E',
    templateUrl: '/static/angular-components/client/virtual-file-system/' +
        'add-item-button.html',
    controller: AddItemButtonController,
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
exports.AddItemButtonDirective.directive_name = 'grrAddItemButton';
