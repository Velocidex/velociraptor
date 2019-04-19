'use strict';

goog.module('grrUi.sidebar.navigatorDirective');
goog.module.declareLegacyNamespace();

const {ApiService, stripTypeInfo} = goog.require('grrUi.core.apiService');
const {RoutingService} = goog.require('grrUi.routing.routingService');


/**
 * Controller for NavigatorDirective.
 *
 * @constructor
 * @param {!angular.Scope} $scope
 * @param {!ApiService} grrApiService
 * @param {!RoutingService} grrRoutingService
 * @ngInject
 */
const NavigatorController = function(
    $scope, grrApiService, grrRoutingService) {
  /** @private {!angular.Scope} */
  this.scope_ = $scope;

  /** @private {!ApiService} */
  this.grrApiService_ = grrApiService;

  /** @private {!RoutingService} */
  this.grrRoutingService_ = grrRoutingService;

  /** @type {Object} */
  this.client;

  /** @type {?string} */
  this.clientId;

  /** @type {boolean} */
  this.hasClientAccess = false;

  /** @type {Object} */
  this.uiTraits;

  this.collapsed = true;

  // Fetch UI traits.
   this.grrApiService_.getCached('v1/GetUserUITraits').then(function (response) {
    this.uiTraits = response['data']['interface_traits'];
   }.bind(this));

  // Subscribe to legacy grr events to be notified on client change.
  this.grrRoutingService_.uiOnParamsChanged(this.scope_, 'clientId',
      this.onClientSelectionChange_.bind(this), true);
};

NavigatorController.prototype.onClientSelectionChange_ = function(clientId) {
  if (!clientId) {
    return; // Stil display the last client for convenience.
  }
  if (clientId.indexOf('aff4:/') === 0) {
    clientId = clientId.split('/')[1];
  }
  if (this.clientId === clientId) {
    return;
  }

  this.clientId = clientId;
};

NavigatorController.prototype.Collapse = function() {
    this.collapsed = !this.collapsed;
};

/**
 * Checks client access.
 *
 * @private
 */
NavigatorController.prototype.checkClientAccess_ = function() {
  this.grrApiService_.head('v1/GetClientFlows/' + this.clientId).then(
      function resolve() {
        this.hasClientAccess = true;
      }.bind(this),
      function reject() {
        this.hasClientAccess = false;
      }.bind(this));
};


/**
 * Directive for the navigator.
 *
 * @return {!angular.Directive} Directive definition object.
 * @ngInject
 * @export
 */
exports.NavigatorDirective = function() {
  return {
    scope: {},
    restrict: 'E',
    templateUrl: '/static/angular-components/sidebar/navigator.html',
    controller: NavigatorController,
    controllerAs: 'controller'
  };
};

/**
 * Directive's name in Angular.
 *
 * @const
 * @export
 */
exports.NavigatorDirective.directive_name = 'grrNavigator';
