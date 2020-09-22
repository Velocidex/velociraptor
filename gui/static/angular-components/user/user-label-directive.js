'use strict';

goog.module('grrUi.user.userLabelDirective');

/**
 * Controller for UserLabelDirective.
 *
 * @param {!angular.Scope} $scope
 * @param {!grrUi.core.apiService.ApiService} grrApiService
 * @constructor
 * @ngInject
 */
const UserLabelController = function($scope, grrApiService) {
  /** @private {!angular.Scope} */
  this.scope_ = $scope;

  /** @private {!grrUi.core.apiService.ApiService} */
  this.grrApiService_ = grrApiService;

  /** @type {string} */
  this.username;
  this.auth_using_google = false;

  /** @type {string} */
  this.user_picture;

  /** @type {string} */
  this.error;

  this.grrApiService_.getCached('v1/GetUserUITraits').then(function(response) {
    this.username = response.data['username'];
    this.auth_using_google = response.data.interface_traits['auth_using_google'];
    this.picture = response.data.interface_traits['picture'];
  }.bind(this), function(error) {
    if (error['status'] == 403) {
      this.error = 'Authentication Error';
    } else {
      this.error = error['statusText'] || ('Error');
    }
  }.bind(this));
};


/**
 * Directive that displays the notification button.
 *
 * @return {!angular.Directive} Directive definition object.
 * @ngInject
 * @export
 */
exports.UserLabelDirective = function() {
  return {
    scope: true,
    restrict: 'E',
    templateUrl: window.base_path+'/static/angular-components/user/user-label.html',
    controller: UserLabelController,
    controllerAs: 'controller'
  };
};

var UserLabelDirective = exports.UserLabelDirective;


/**
 * Name of the directive in Angular.
 *
 * @const
 * @export
 */
UserLabelDirective.directive_name = 'grrUserLabel';
