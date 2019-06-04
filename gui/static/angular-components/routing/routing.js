'use strict';

goog.module('grrUi.routing.routing');

const {RoutingService} = goog.require('grrUi.routing.routingService');
const {encodeUrlPath} = goog.require('grrUi.core.apiService');
const {rewriteUrl} = goog.require('grrUi.routing.rewriteUrl');

/**
 * Angular module for core routing components.
 */
exports.routingModule = angular.module('grrUi.routing', ['ui.router']);

exports.routingModule.service(RoutingService.service_name, RoutingService);

exports.routingModule
  .config(function(
    $stateProvider, $urlRouterProvider, $urlMatcherFactoryProvider) {
    $urlMatcherFactoryProvider.type('pathWithUnescapedSlashes', {
      encode: function(item) {
        if (!item) {
          return '';
        } else {
          return encodeUrlPath(item);
        }
      },
      decode: function(item) {
        if (!item) {
          return '';
        } else {
          return decodeURIComponent(item);
        }
      },
      pattern: /.*/
    });

    // Prevent $urlRouter from automatically intercepting URL changes;
    // this allows you to configure custom behavior in between
    // location changes and route synchronization:
    $urlRouterProvider.deferIntercept();

    // Default route.
    //$urlRouterProvider.otherwise('/');

    // We use inline templates to let our routes directly point to directives.
    $stateProvider

    //
    // Landing state.
    //
      .state('userDashboard', {
        url: '',
        template: '<grr-user-dashboard />',
        title: 'Home',
      })
      .state('home', {
        url: '',
        template: '<grr-user-dashboard />',
        title: 'Home',
      })
      .state('search', {
        url: '/search?q',
        template: '<grr-clients-list />',
        title: function(params) {
          if (params['q']) {
            return 'Search for "' + params['q'] + '"';
          } else {
            return 'Client List';
          }
        }
      })

      .state('hunts', {
        url: '/hunts/:huntId/:tab',
        template: '<grr-hunts-view />',
        params: {
          huntId: {value: null, squash: true},
          tab: {value: null, squash: true}
        },
        title: function(params) {
          if (params['huntId']) {
            return params['huntId'];
          } else {
            return 'Hunts';
          }
        }
      })
      .state('view_artifacts', {
        url: '/view_artifacts',
        template: '<grr-artifact-viewer />',
        title: 'View Artifacts'
      })

      .state('server_events', {
        url: '/server_events',
        template: '<grr-server-events />',
        title: "Server Events",
      })
      .state('server_artifacts', {
        url: '/server_artifacts',
        template: '<grr-server-artifacts />',
        title: "Server Artifacts",
      })
      .state('alerts', {
        url: '/alerts',
        template: '<grr-alerts />',
        title: "Alerts",
      })

    //
    // States when a client is selected.
    //
      .state('client', {
        url: '/clients/:clientId',
        redirectTo: 'client.hostInfo',
        template: '<div ui-view></div>',
        title: function(params) {
          return params['clientId'];
        }
      })
      .state('client.hostInfo', {
        url: '/host-info',
        template: '<grr-host-info />',
        title: 'Host Information'
      })
      .state('client.vfs', {
        url: '/vfs/{path:pathWithUnescapedSlashes}?version&mode&tab',
        template: '<grr-file-view />',
        title: function(params) {
          return '/' + (params['path'] || '');
        }
      })
      .state('client.flows', {
        url: '/flows/:flowId/:tab',
        template: '<grr-client-flows-view />',
        params: {
          flowId: {value: null, squash: true},
          tab: {value: null, squash: true}
        },
        title: function(params) {
          if (params['flowId']) {
            return params['flowId'];
          } else {
            return 'Flows';
          }
        }
      })
      .state('client.events', {
        url: '/client_events',
        template: '<grr-client-events />',
        title: "Client Events",
      });
  })
  .run(function($rootScope, $location, $state, $urlRouter, $document) {
    /**
     * This function builds page title based on the current state.
     */
    var updateTitle = function() {
      var breadcrumbs = [];
      var curState = $state['$current'];
      while (angular.isDefined(curState)) {
        if (angular.isString(curState.title)) {
          breadcrumbs.splice(0, 0, curState.title);
        } else if (angular.isFunction(curState.title)) {
          var newItem = curState.title($state.params);
          if (angular.isArray(newItem)) {
            breadcrumbs = newItem.concat(breadcrumbs);
          } else {
            breadcrumbs.splice(0, 0, newItem);
          }
        }

        curState = curState.parent;
      }

      breadcrumbs.splice(0, 0, 'Velociraptor');
      $document[0].title = breadcrumbs.join(' | ');
    };

    // This is the suggested way to implement a default child state for
    // abstract parent states atm. Source:
    // https://github.com/angular-ui/ui-router/issues/948#issuecomment-75342784
    //
    // TODO(user): This can be replaced with AngularUI Routers 1.0+
    // feature of specifying a default child state within the abstract
    // attribute once AngularUI Router 1.0+ is available.
    $rootScope.$on('$stateChangeStart', function(evt, to, params) {
      if (to.redirectTo) {
        evt.preventDefault();
        $state.go(to.redirectTo, params);
      }
    });

    $rootScope.$on('$stateChangeSuccess', updateTitle);

    $rootScope.$on('$locationChangeSuccess', function(evt) {
      // Prevent $urlRouter's default handler from firing.
      evt.preventDefault();

      // Try to rewrite URL.
      var url = $location.url().substring(1);
      var rewrittenUrl = rewriteUrl(url);
      if (rewrittenUrl) {
        $location.url(rewrittenUrl);
      }

      $urlRouter.sync();
      // We need to update the title not only when the state changes
      // (see $stateChangeSuccess), but also when individual state's
      // parameters get updated.
      updateTitle();
    });

    // Configures $urlRouter's listener *after* your custom listener
    $urlRouter.listen();
  });
