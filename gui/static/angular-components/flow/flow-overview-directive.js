'use strict';

goog.module('grrUi.flow.flowOverviewDirective');
goog.module.declareLegacyNamespace();


/**
 * Controller for FlowOverviewDirective.
 *
 * @constructor
 * @param {!angular.Scope} $scope
 * @param {!grrUi.core.apiService.ApiService} grrApiService
 * @ngInject
 */
const FlowOverviewController = function(
    $scope, $rootScope, grrApiService) {
  /** @private {!angular.Scope} */
    this.scope_ = $scope;
    this.grrApiService_ = grrApiService;
    this.rootScope_ = $rootScope;

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


FlowOverviewController.prototype.prepareDownload = function() {
  // Sanitize filename for download.
  var flow = this.scope_["flow"]["context"];
  var url = 'v1/CreateDownload';
  var params = {
    flow_id: flow['session_id'],
    client_id: flow["request"]['client_id'],
  };
  this.grrApiService_.post(url, params).then(
        function success() {}.bind(this)
    );
};

/**
 * Directive for displaying log records of a flow with a given URN.
 *
 * @return {angular.Directive} Directive definition object.
 * @ngInject
 * @export
 */
exports.FlowOverviewDirective = function() {
  return {
      scope: {
          flow: '=',
          flowId: '=',
          apiBasePath: '='
      },
      restrict: 'E',
      templateUrl: '/static/angular-components/flow/flow-overview.html',
      controller: FlowOverviewController,
      controllerAs: 'controller'
  };
};


/**
 * Directive's name in Angular.
 *
 * @const
 * @export
 */
exports.FlowOverviewDirective.directive_name = 'grrFlowOverview';
