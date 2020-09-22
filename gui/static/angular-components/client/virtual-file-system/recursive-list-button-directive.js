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
 * @ngInject
 */
const RecursiveListButtonController = function(
    $rootScope, $scope, $timeout, $uibModal, grrApiService) {
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

    this.refreshOperation = {
        client_id: this.fileContext.clientId,
        'vfs_path': this.fileContext['selectedDirPath'] || '',
        'depth': RecursiveListButtonController.MAX_DEPTH};

    this.modalInstance = this.uibModal_.open({
        templateUrl: window.base_path+'/static/angular-components/client/virtual-file-system/' +
            'recursive-list-button-modal.html',
        scope: this.scope_
    });
};


/**
 * Sends current settings value to the server and reloads the page after as
 * soon as server acknowledges the request afer a small delay. The delay is
 * needed so that the user can see the success message.
 *
 * @export
 */
RecursiveListButtonController.prototype.createRefreshOperation = function() {
    var clientId = this.scope_['clientId'];
    var self = this;
    var url = 'v1/VFSRefreshDirectory';

    // Setting this.lastOperationId means that the update button will get
    // disabled immediately.
    var operationId = self.lastOperationId = 'unknown';
    self.grrApiService_.post(url, self.refreshOperation)
        .then(function success(response) {
            self.done = true;

            // Make modal dialog go away in 1 second.
            self.timeout_(function() {
                self.modalInstance.close();
            }, 1000);

            operationId = self.lastOperationId =
                response['data']['flow_id'];

            var pollPromise = self.grrApiService_.poll('v1/GetFlowDetails/',
                OPERATION_POLL_INTERVAL_MS, {
                    flow_id: operationId,
                    client_id: clientId,
                }, function(response) {
                    return response.data.context.state != 'RUNNING';
                });

            self.scope_.$on('$destroy', function() {
                self.grrApiService_.cancelPoll(pollPromise);
            });

            return pollPromise;
        }, function failure(response) {
            self.done = true;
            self.error = response['data']['message'] || 'Unknown error.';
        }).then(function success() {
            var path = self.refreshOperation.vfs_path;
            self.rootScope_.$broadcast(
                REFRESH_FOLDER_EVENT, ensurePathIsFolder(path));
        }).finally(function() {
            if (self.lastOperationId == operationId) {
                self.lastOperationId = null;
            }
        });
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
    templateUrl: window.base_path+'/static/angular-components/client/virtual-file-system/' +
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
