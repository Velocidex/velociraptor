'use strict';

goog.module('grrUi.user.userDashboardDirective');
goog.module.declareLegacyNamespace();



/**
 * Controller for UserDashboardDirective.
 *
 * @constructor
 * @param {!angular.Scope} $scope
 * @param {!grrUi.core.apiService.ApiService} grrApiService
 * @param {!grrUi.routing.routingService.RoutingService} grrRoutingService
 * @ngInject
 */
const UserDashboardController = function(
    $scope, grrApiService, grrRoutingService) {
  /** @private {!angular.Scope} */
  this.scope_ = $scope;

  /** @private {!grrUi.core.apiService.ApiService} */
  this.grrApiService_ = grrApiService;

  /** @private {!grrUi.routing.routingService.RoutingService} */
  this.grrRoutingService_ = grrRoutingService;

  let current_datetime = new Date();
  this.params = {
    artifact: "Server.Monitor.Health",
    type: "SERVER_EVENT",
    dayName: moment.utc().format("YYYY-MM-DD")
  };
};

/**
 * UserDashboardDirective renders a dashboard that users see when they
 * hit GRR's Admin UI home page.
 *
 * @return {!angular.Directive} Directive definition object.
 */
exports.UserDashboardDirective = function() {
  return {
    scope: {},
    restrict: 'E',
    templateUrl: '/static/angular-components/user/user-dashboard.html',
    controller: UserDashboardController,
    controllerAs: 'controller'
  };
};


/**
 * Name of the directive in Angular.
 *
 * @const
 * @export
 */
exports.UserDashboardDirective.directive_name = 'grrUserDashboard';
