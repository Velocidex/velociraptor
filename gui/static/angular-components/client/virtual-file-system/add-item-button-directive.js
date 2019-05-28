'use strict';

goog.module('grrUi.client.virtualFileSystem.addItemButtonDirective');
goog.module.declareLegacyNamespace();


/**
 * Controller for AddItemButtonController
 *
 * @param {!angular.Scope} $rootScope
 * @param {!angular.Scope} $scope
 * @param {!grrUi.core.apiService.ApiService} grrApiService
 * @constructor
 * @ngInject
 */
const AddItemButtonController = function(
    $rootScope, $scope, $uibModal, grrApiService) {
  /** @private {!angular.Scope} */
  this.rootScope_ = $rootScope;

  /** @private {!angular.Scope} */
  this.scope_ = $scope;

  /** @private {!angularUi.$uibModal} */
  this.uibModal_ = $uibModal;

  /** @private {!grrUi.core.apiService.ApiService} */
  this.grrApiService_ = grrApiService;

  /** @type {?string} */
  this.lastOperationId;

  /** @type {Object} */
  this.refreshOperation;

    this.names = [];
    this.params = {};

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
    this.value;

    // Used to add server monitoring artifacts.
    this.flowArguments = {};
};

AddItemButtonController.prototype.onClientOrPathChange_ = function() {
    return;

    var self =this;

    var is_valid_path = function(path) {
        if (angular.isUndefined(path)) {
            return false;
        };

        path = path.replace(/\/+/g, "/");

        // Manipulate artifact definitions.
        if (path.startsWith("/artifact_definitions/")) {
            path = path.replace(/builtin/, "custom");

            return path;
        } else if (path.startsWith("/server_artifacts/")) {
            return path;

        } else {
            return false;
        }
    };

    this.path = is_valid_path(this.scope_.dirPath) ||
        is_valid_path(this.scope_.filePath);
};

AddItemButtonController.prototype.updateArtifactDefinitions = function() {
    var url = 'v1/GetArtifactFile';
    var params = {
        vfs_path: this.scope_.filePath,
    };
    var self = this;

    this.error = "";
    this.grrApiService_.get(url, params).then(function(response) {
        self.value = response['data']['artifact'];
        self.modalInstance = self.uibModal_.open({
            templateUrl: '/static/angular-components/artifact/add_artifact.html',
            scope: self.scope_,
            size: "lg",
        });
    });

    return false;
};


AddItemButtonController.prototype.saveArtifact = function() {
    var url = 'v1/SetArtifactFile';
    var params = {
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


AddItemButtonController.prototype.updateServerMonitoringTable = function() {
    var url = 'v1/GetServerMonitoringState';
    var self = this;

    this.error = "";
    this.grrApiService_.get(url).then(function(response) {
        self.flowArguments = response['data'];
        if (angular.isObject(self.flowArguments.artifacts)) {
            self.names = self.flowArguments.artifacts.names || [];
            self.params = {};
            var parameters = self.flowArguments.parameters.env || {};
            for (var i=0; i<parameters.length;i++) {
                var p = parameters[0];
                self.params[p["key"]] = p["value"];
            }
        }
        self.modalInstance = self.uibModal_.open({
            templateUrl: '/static/angular-components/artifact/add_server_monitoring.html',
            scope: self.scope_,
            size: "lg",
        });
    });
    return false;
};

AddItemButtonController.prototype.saveServerArtifacts = function() {
    var self = this;

    // Update the names and the parameters.
    var env = [];
    for (var k in self.params) {
        if (self.params.hasOwnProperty(k)) {
            env.push({key: k, value: self.params[k]});
        }
    }

    self.flowArguments.artifacts = {names: self.names};
    self.flowArguments.parameters = {env: env};

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

AddItemButtonController.prototype.updateClientMonitoringTable = function() {
    var url = 'v1/GetClientMonitoringState';
    var self = this;

    this.error = "";
    this.grrApiService_.get(url).then(function(response) {
        self.flowArguments = response['data'];
        self.names = self.flowArguments.artifacts.names || [];
        self.modalInstance = self.uibModal_.open({
            templateUrl: '/static/angular-components/artifact/add_client_monitoring.html',
            scope: self.scope_,
            size: "lg",
        });
    });
    return false;
};

AddItemButtonController.prototype.saveClientMonitoringArtifacts = function() {
    var self = this;

    // Update the names and the parameters.
    var env = [];
    for (var k in self.params) {
        if (self.params.hasOwnProperty(k)) {
            env.push({key: k, value: self.params[k]});
        }
    }
    self.flowArguments.artifacts.names = self.names;
    self.flowArguments.parameters = {env: env};

    var url = 'v1/SetClientMonitoringState';
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

exports.AddItemButtonDirective = function() {
  return {
    scope: {
        clientId: '=',
        filePath: '=',
        dirPath: '=',
        mode: '=',
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
