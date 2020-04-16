'use strict';

goog.module('grrUi.user.userNotificationItemDirective');
goog.module.declareLegacyNamespace();

const {encodeUrlPath} = goog.require('grrUi.core.apiService');
const {getLastPathComponent, stripAff4Prefix} = goog.require('grrUi.core.utils');


/**
 * Opens the reference of a notification.
 *
 * @param {Object} notification
 * @param {!angular.$location} $location
 * @return {boolean} Returns true if the location was changed.
 *
 * @export
 */
exports.openReference = function(notification, $location) {
  if (!notification['isFileDownload'] && notification['link']) {
    $location.path(notification['link']);
    return true;
  } else {
    return false;
  }
};
var openReference = exports.openReference;

/**
 * Prepares the notification for displaying.
 *
 * @param {Object} notification
 */
exports.annotateApiNotification = function(notification) {
  if (angular.isDefined(notification['reference'])) {
    notification['link'] = getLink_(notification);
  }
};
var annotateApiNotification = exports.annotateApiNotification;

/**
 * Creates a link for the notification.
 *
 * @param {Object} notification The notification.
 * @return {Object<string, string>|string} The URL parameters or the URL
 * path for the given notification.
 *
 * @private
 */
var getLink_ = function(notification) {
  var strippedNotification = notification;
  if (!strippedNotification['reference']){
    return null;
  }

  var reference = strippedNotification['reference'];
  var urlParameters = {};

  if (angular.isDefined(reference.hunt)) {
    var huntId = getLastPathComponent(reference.hunt['hunt_urn']);
    return ['hunts',
            huntId].join('/');
  } else if (angular.isDefined(reference.vfs_file)) {
    return ['clients',
            stripAff4Prefix(reference.vfs_file['client_id']),
            'vfs',
            encodeUrlPath(stripAff4Prefix(reference.vfs_file['vfs_path']))].join('/');
  } else if (angular.isDefined(reference.flow)) {
    var flowId = reference.flow['flow_id'];
    return ['clients',
            stripAff4Prefix(reference.flow['client_id']),
            'flows',
            flowId].join('/');
  } else if (angular.isDefined(reference.approval_request)) {
    var clientId = stripAff4Prefix(reference.approval_request['client_id']);
    return ['users',
            reference.approval_request['username'],
            'approvals',
            'client',
            clientId,
            reference.approval_request['approval_id']].join('/');
  }

  return null;
};


/**
 * Controller for UserNotificationItemDirective.
 *
 * @param {!angular.Scope} $scope
 * @param {!angular.$location} $location
 * @constructor
 * @ngInject
 */
const UserNotificationItemController =
  function($scope, $location) {
  /** @private {!angular.Scope} */
  this.scope_ = $scope;

  /** @private {!angular.$location} */
  this.location_ = $location;

  this.scope_.$watch('notification', this.onNotificationChanged_.bind(this));
};



/**
 * Prepares the notification for displaying.
 *
 * @param {Object} notification
 * @private
 */
UserNotificationItemController.prototype.onNotificationChanged_ = function(
    notification) {
  annotateApiNotification(notification);
};

/**
 * Opens the reference of the notification.
 *
 * @export
 */
UserNotificationItemController.prototype.openReference = function() {
  if (openReference(this.scope_['notification'], this.location_)) {
    this.scope_['close']();
  }
};


/**
 * Directive for showing a notification.
 *
 * @return {!angular.Directive} Directive definition object.
 * @ngInject
 * @export
 */
exports.UserNotificationItemDirective = function() {
  return {
    scope: {
      notification: '=',
      close: '&'
    },
    restrict: 'E',
    templateUrl: '/static/angular-components/user/user-notification-item.html',
    controller: UserNotificationItemController,
    controllerAs: 'controller'
  };
};


/**
 * Directive's name in Angular.
 *
 * @const
 * @export
 */
exports.UserNotificationItemDirective.directive_name =
    'grrUserNotificationItem';
