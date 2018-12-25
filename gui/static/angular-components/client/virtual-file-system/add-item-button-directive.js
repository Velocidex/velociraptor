'use strict';

goog.module('grrUi.client.virtualFileSystem.addItemButtonDirective');
goog.module.declareLegacyNamespace();


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
  this.fileContext;

    this.scope_.$watchGroup(['clientId', 'filePath', 'dirPath'],
                            this.onClientOrPathChange_.bind(this));

    this.path;
    this.value;
    this.isActive = false;
};

AddItemButtonController.prototype.onClientOrPathChange_ = function() {
    console.log(this.scope_.filePath);
    console.log(this.scope_.dirPath);

    var self =this;

    var is_valid_path = function(path) {
        if (!angular.isDefined(path)) {
            return false;
        };

        path = path.replace(/\/+/g, "/");
        path = path.replace(/builtin/, "custom");

        if (!path.startsWith("/artifact_definitions/")) {
            return false;
        }

        self.isActive = true;
        return path;
    };

    this.isActive = false;
    this.path = is_valid_path(this.scope_.filePath) ||
        is_valid_path(this.scope_.dirPath);
};


AddItemButtonController.prototype.onClick = function() {
    var url = 'v1/GetArtifactFile';
    var params = {
        vfs_path: this.scope_.filePath,
    };
    this.error = "";
    this.grrApiService_.get(url, params).then(function(response) {
        this.value = response['data']['artifact'];
        this.modalInstance = this.uibModal_.open({
            templateUrl: '/static/angular-components/artifact/add_artifact.html',
            scope: this.scope_,
            size: "lg",
        });

    }.bind(this), function() {

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
