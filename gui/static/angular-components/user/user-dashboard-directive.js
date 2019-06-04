'use strict';

goog.module('grrUi.user.userDashboardDirective');

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
  };

  this.ranges = [
    {desc: "Last Day", sec: 60*60*24, sample: 4},
    {desc: "Last 2 days", sec: 60*60*24*2, sample: 8},
    {desc: "Last Week", sec: 60*60*24*7, sample: 40},
  ];

  this.current_range_desc;
  this.setRange(this.ranges[0]);
};

UserDashboardController.prototype.setRange = function(args) {
  this.params.start_time = moment.utc().unix() - args.sec;
  this.params.sample = args.sample;
  this.current_range_desc = args.desc;
};

exports.UserDashboardDirective = function() {
  return {
    scope: {},
    restrict: 'E',
    templateUrl: '/static/angular-components/user/user-dashboard.html',
    controller: UserDashboardController,
    controllerAs: 'controller'
  };
};

exports.UserDashboardDirective.directive_name = 'grrUserDashboard';
