'use strict';

goog.module('grrUi.client.virtualFileSystem.recursiveListButtonDirective');
goog.module.declareLegacyNamespace();

const {REFRESH_FOLDER_EVENT} = goog.require('grrUi.client.virtualFileSystem.events');
const {ensurePathIsFolder, getFolderFromPath} = goog.require('grrUi.client.virtualFileSystem.utils');



var OPERATION_POLL_INTERVAL_MS = 1000;



/**
 * Controller for RecursiveListButtonDirective.
 *
 * @constructor
 * @param {!angular.Scope} $rootScope
 * @param {!angular.Scope} $scope
 * @param {!angular.$timeout} $timeout
 * @param {!angularUi.$uibModal} $uibModal Bootstrap UI modal service.
 * @param {!grrUi.core.apiService.ApiService} grrApiService
 * @param {!grrUi.core.reflectionService.ReflectionService} grrReflectionService
 * @ngInject
 */
const RecursiveListButtonController = function(
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

    /** @private {object} */
    this.fileContext;

    this.scope_.$watchGroup(['controller.fileContext.clientId',
                             'controller.fileContext.selectedDirPath'],
                            this.onClientOrPathChange_.bind(this));
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
 * @const {number}
 */
RecursiveListButtonController.MAX_DEPTH = 5;


/**
 * Handles changes in clientId and filePath bindings.
 *
 * @private
 */
RecursiveListButtonController.prototype.onClientOrPathChange_ = function() {
  this.lastOperationId = null;
};


/**
 * Handles mouse clicks on itself.
 *
 * @export
 */
RecursiveListButtonController.prototype.onClick = function() {
  this.isDisabled = true;
  this.done = false;
  this.error = null;

  this.grrReflectionService_.getRDFValueDescriptor(
    'VFSRefreshDirectoryRequest', true).then(function(descriptors) {
      this.refreshOperation = {
        'vfs_path': this.fileContext['selectedDirPath'] || '',
        'depth': RecursiveListButtonController.MAX_DEPTH};

      this.modalInstance = this.uibModal_.open({
        templateUrl: '/static/angular-components/client/virtual-file-system/' +
          'recursive-list-button-modal.html',
        scope: this.scope_
      });
    }.bind(this));
};


/**
 * Sends current settings value to the server and reloads the page after as
 * soon as server acknowledges the request afer a small delay. The delay is
 * needed so that the user can see the success message.
 *
 * @export
 */
RecursiveListButtonController.prototype.createRefreshOperation = function() {
  var aff4Prefix = 'aff4:/';
  var clientId = this.scope_['clientId'];
  if (clientId.toLowerCase().indexOf(aff4Prefix) == 0) {
    clientId = clientId.substr(aff4Prefix.length);
  }

  var url = 'v1/VFSRefreshDirectory/' + clientId;

  // Setting this.lastOperationId means that the update button will get
  // disabled immediately.
  var operationId = this.lastOperationId = 'unknown';
  this.grrApiService_.post(url, this.refreshOperation, true)
      .then(
          function success(response) {
            this.done = true;
            // Make modal dialog go away in 1 second.
            this.timeout_(function() {
              this.modalInstance.close();
            }.bind(this), 1000);

            operationId = this.lastOperationId =
                response['data']['flow_id'];

            var pollPromise = this.grrApiService_.poll(
              'v1/GetFlowDetails/' + clientId,
              OPERATION_POLL_INTERVAL_MS, {
                'flow_id': operationId,
              }, function(response) {
                return response.data.context.state != 'RUNNING';
              });
            this.scope_.$on('$destroy', function() {
              this.grrApiService_.cancelPoll(pollPromise);
            }.bind(this));

            return pollPromise;
          }.bind(this),
          function failure(response) {
            this.done = true;
            this.error = response['data']['message'] || 'Unknown error.';
          }.bind(this))
      .then(
          function success() {
            var path = this.refreshOperation.vfs_path;
            this.rootScope_.$broadcast(
                REFRESH_FOLDER_EVENT, ensurePathIsFolder(path));
          }.bind(this))
      .finally(function() {
        if (this.lastOperationId == operationId) {
          this.lastOperationId = null;
        }
      }.bind(this));
};

/**
 * RecursiveListButtonDirective renders a button that shows a dialog that allows
 * users to change their personal settings.
 *
 * @return {!angular.Directive} Directive definition object.
 */
exports.RecursiveListButtonDirective = function() {
  return {
    scope: {
      clientId: '=',
      filePath: '='
    },
    require: '^grrFileContext',
    restrict: 'E',
    templateUrl: '/static/angular-components/client/virtual-file-system/' +
        'recursive-list-button.html',
    controller: RecursiveListButtonController,
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
exports.RecursiveListButtonDirective.directive_name = 'grrRecursiveListButton';
