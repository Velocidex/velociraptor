'use strict';

goog.module('grrUi.hunt.huntsListDirective');
goog.module.declareLegacyNamespace();

//const {AclDialogService} = goog.require('grrUi.acl.aclDialogService');
const {ApiService} = goog.require('grrUi.core.apiService');
const {DialogService} = goog.require('grrUi.core.dialogService');
const {stripAff4Prefix} = goog.require('grrUi.core.utils');


/**
 * Controller for HuntsListDirective.
 *
 * @constructor
 * @param {!angular.Scope} $scope
 * @param {!angular.$q} $q
 * @param {!angularUi.$uibModal} $uibModal Bootstrap UI modal service.
 * @param {DialogService} grrDialogService
 * @param {!ApiService} grrApiService
 * @ngInject
 */
const HuntsListController = function(
    $scope, $q, $uibModal, grrDialogService, grrApiService) {
    // Injected dependencies.

    /** @private {!angular.Scope} */
    this.scope_ = $scope;

    /** @private {!angular.$q} */
    this.q_ = $q;

    /** @private {!angularUi.$uibModal} */
    this.uibModal_ = $uibModal;

    /** @private {DialogService} */
    this.grrDialogService_ = grrDialogService;

    /** @private {!ApiService} */
    this.grrApiService_ = grrApiService;

    /** @type {string} */
    this.selectedHunt;

    // Internal state.
    /**
     * This variable is bound to grr-infinite-table's trigger-update attribute
     * and therefore is set by that directive to a function that triggers
     * table update.
     * @export {function()}
     */
    this.triggerUpdate;

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
 * Hunts list API url.
 * @const {string}
 */
HuntsListController.prototype.huntsUrl = 'v1/ListHunts';


/**
 * Computes an URL to currently selected hunt.
 *
 * @return {string} URL to the selected hunt.
 *
 * @private
 */
HuntsListController.prototype.buildHuntUrl_ = function() {
  var components = this.scope_['selectedHuntId'].split('/');
  var basename = components[components.length - 1];
  return this.huntsUrl + '/' + basename;
};


/*
 * TODO(hanuszczak): The method below looks like a duplication with
 * `CronJobsListController.wrapApiPromise_`. Maybe these can be merged into one
 * method instead?
 */

/**
 * @param {!angular.$q.Promise} promise A promise to wrap.
 * @param {string} successMessage Message to return on success.
 * @return {!angular.$q.Promise} Wrapped promise.
 *
 * @private
 */
HuntsListController.prototype.wrapApiPromise_ = function(promise, successMessage) {
    return promise.then(
        function success() {
          return successMessage;
        }.bind(this),
        function failure(response) {
          var message = response['data'];
          return this.q_.reject(message);
        }.bind(this));
};

/**
 * Selects given item in the list.
 *
 * @param {!Object} item Item to be selected.
 * @export
 * @suppress {missingProperties} For items, as they crom from JSON response.
 */
HuntsListController.prototype.selectItem = function(item) {
  this.scope_['selectedHuntId'] = item.hunt_id;
  this.selectedHunt = item;
};

HuntsListController.prototype.huntState = function(item) {
    if (angular.isObject(item)){
        if (item.stats.stopped) {
            return "STOPPED";
        }

        return item.state;
    }
};


/**
 * Shows new hunt wizard.
 *
 * @export
 */
HuntsListController.prototype.newHunt = function() {
  var modalScope = this.scope_.$new();
  modalScope.resolve = function() {
    modalInstance.close();
  };
  modalScope.reject = function() {
    modalInstance.dismiss();
  };
  this.scope_.$on('$destroy', function() {
    modalScope.$destroy();
  });

  var modalInstance = this.uibModal_.open({
    template: '<grr-new-hunt-wizard-form on-resolve="resolve()" ' +
        'on-reject="reject()" />',
    scope: modalScope,
    windowClass: 'wide-modal high-modal',
    size: 'lg'
  });

  modalInstance.result.then(function resolve() {
    this.triggerUpdate();
  }.bind(this));
};


/**
 * Shows 'Run Hunt' confirmation dialog.
 *
 * @export
 */
HuntsListController.prototype.runHunt = function() {
  var modalPromise = this.grrDialogService_.openConfirmation(
    'Run this hunt?',
    'Are you sure you want to run this hunt?',
      function() {
        var promise = this.grrApiService_.post(
          'v1/ModifyHunt', {state: 'RUNNING', hunt_id: this.scope_['selectedHuntId']});
        return this.wrapApiPromise_(promise, 'Hunt started successfully!');
      }.bind(this));
  modalPromise.then(function resolve() {
    this.triggerUpdate();
  }.bind(this), function dismiss() {
    this.triggerUpdate();
  }.bind(this));
};


/**
 * Shows 'Stop Hunt' confirmation dialog.
 *
 * @export
 */
HuntsListController.prototype.stopHunt = function() {
  var modalPromise = this.grrDialogService_.openConfirmation(
      'Stop this hunt?',
      'Stopped hunts can not be restarted. You can create a new hunt later.',
      function() {
        var promise = this.grrApiService_.post(
          'v1/ModifyHunt', {state: 'PAUSED', hunt_id: this.scope_['selectedHuntId']});
        return this.wrapApiPromise_(promise, 'Hunt stopped successfully!');
      }.bind(this));
  modalPromise.then(function resolve() {
    this.triggerUpdate();
  }.bind(this), function dismiss() {
    this.triggerUpdate();
  }.bind(this));
};


/**
 * Shows 'Modify Hunt' confirmation dialog.
 *
 * @export
 */
HuntsListController.prototype.modifyHunt = function() {
  var components = this.scope_['selectedHuntId'].split('/');
  var huntId = components[components.length - 1];

  var argsObj = {};
  var modalPromise = this.grrDialogService_.openDirectiveDialog(
    'grrModifyHuntDialog', { huntId: huntId });

  // TODO(user): there's no need to trigger update on dismiss.
  // Doing so only to maintain compatibility with legacy GRR code.
  // Remove as soon as legacy GRR code is removed.
  modalPromise.then(function resolve() {
    this.triggerUpdate();
  }.bind(this), function dismiss() {
    this.triggerUpdate();
  }.bind(this));
};


/**
 * Shows 'New Hunt' dialog prefilled with the data of the currently selected
 * hunt.
 *
 * @export
 */
HuntsListController.prototype.copyHunt = function() {
  var modalScope = this.scope_.$new();
  modalScope.huntId = this.scope_['selectedHuntId'];
  modalScope.resolve = function() {
    modalInstance.close();
  };
  modalScope.reject = function() {
    modalInstance.dismiss();
  };

  this.scope_.$on('$destroy', function() {
    modalScope.$destroy();
  });

  var modalInstance = this.uibModal_.open({
    template: '<grr-new-hunt-wizard-copy-form on-resolve="resolve()" ' +
        'on-reject="reject()" hunt-id="huntId" />',
    scope: modalScope,
    windowClass: 'wide-modal high-modal',
    size: 'lg'
  });
  modalInstance.result.then(function resolve() {
    this.triggerUpdate();
  }.bind(this));
};


/**
 * Shows 'Delete Hunt' confirmation dialog.
 *
 * @export
 */
HuntsListController.prototype.deleteHunt = function() {
  var modalPromise = this.grrDialogService_.openConfirmation(
      'Delete this hunt?',
      'Are you sure you want to delete this hunt?',
    function() {
      var selectedHuntId = this.scope_['selectedHuntId'];
      this.scope_['selectedHuntId'] = "";

      var promise = this.grrApiService_.post(
        'v1/ModifyHunt', {state: 'ARCHIVED', hunt_id: selectedHuntId});


      return this.wrapApiPromise_(promise, 'Hunt archived successfully!');
    }.bind(this));

  // TODO(user): there's no need to trigger update on dismiss.
  // Doing so only to maintain compatibility with legacy GRR code.
  // Remove as soon as legacy GRR code is removed.
  modalPromise.then(function resolve() {
    this.triggerUpdate();
  }.bind(this), function dismiss() {
    this.triggerUpdate();
  }.bind(this));
};


/**
 * Displays a table with list of available hunts.
 *
 * @return {angular.Directive} Directive definition object.
 */
exports.HuntsListDirective = function() {
  return {
    scope: {
      selectedHuntId: '=?',
    },
    restrict: 'E',
    templateUrl: window.base_path+'/static/angular-components/hunt/hunts-list.html',
    controller: HuntsListController,
    controllerAs: 'controller'
  };
};


/**
 * Directive's name in Angular.
 *
 * @const
 * @export
 */
exports.HuntsListDirective.directive_name = 'grrHuntsList';
