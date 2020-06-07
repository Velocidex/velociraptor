'use strict';

goog.module('grrUi.user.user');
goog.module.declareLegacyNamespace();

const {UserDashboardDirective} = goog.require('grrUi.user.userDashboardDirective');
const {UserDesktopNotificationsDirective} = goog.require('grrUi.user.userDesktopNotificationsDirective');
const {UserNotificationButtonDirective} = goog.require('grrUi.user.userNotificationButtonDirective');
const {UserLabelDirective} = goog.require('grrUi.user.userLabelDirective');
const {UserNotificationDialogDirective} = goog.require('grrUi.user.userNotificationDialogDirective');
const {UserNotificationItemDirective} = goog.require('grrUi.user.userNotificationItemDirective');
const {coreModule} = goog.require('grrUi.core.core');


/**
 * Angular module for user-related UI.
 */
exports.userModule =
    angular.module('grrUi.user', [coreModule.name]);

exports.userModule.directive(
  UserLabelDirective.directive_name, UserLabelDirective);
exports.userModule.directive(
    UserDashboardDirective.directive_name, UserDashboardDirective);
exports.userModule.directive(
    UserDesktopNotificationsDirective.directive_name,
    UserDesktopNotificationsDirective);
exports.userModule.directive(
    UserNotificationButtonDirective.directive_name,
    UserNotificationButtonDirective);
exports.userModule.directive(
    UserNotificationDialogDirective.directive_name,
    UserNotificationDialogDirective);
exports.userModule.directive(
    UserNotificationItemDirective.directive_name,
    UserNotificationItemDirective);
