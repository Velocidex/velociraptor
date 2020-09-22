'use strict';

goog.module('grrUi.user.userNotificationDialogDirective');
goog.module.declareLegacyNamespace();



/**
 * Controller for UserNotificationDialogDirective.
 *
 * @param {!angular.Scope} $scope
 * @constructor
 * @ngInject
 */
const UserNotificationDialogController =
  function($scope) {
  /** @private {!angular.Scope} */
  this.scope_ = $scope;

  /** @type {string} */
    this.notificationUrl = 'v1/GetUserNotifications';
    this.notificationParams = {
      clear_pending: true,
    };
};



/**
 * Directive for showing the notification dialog.
 *
 * @return {!angular.Directive} Directive definition object.
 * @ngInject
 * @export
 */
exports.UserNotificationDialogDirective = function() {
  return {
    scope: {
      close: '&'
    },
    restrict: 'E',
    templateUrl: window.base_path+'/static/angular-components/user/user-notification-dialog.html',
    controller: UserNotificationDialogController,
    controllerAs: 'controller'
  };
};


/**
 * Directive's name in Angular.
 *
 * @const
 * @export
 */
exports.UserNotificationDialogDirective.directive_name =
    'grrUserNotificationDialog';
