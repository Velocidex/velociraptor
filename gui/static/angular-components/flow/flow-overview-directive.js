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
};


/**
 * Downloads the file.
 *
 * @export
 */
FlowOverviewController.prototype.downloadFile = function() {
    var flow = this.scope_["flow"];
    var clientId = flow['client_id'];
    var flow_id = flow['flow_id'];
    var filename = clientId + '/' + flow_id;

    // Sanitize filename for download.
    var url = 'v1/download/' + filename.replace(/[^./a-zA-Z0-9]+/g, '_');
    this.grrApiService_.downloadFile(url).then(
        function success() {}.bind(this),
        function failure(response) {
            if (angular.isUndefined(response.status)) {
                this.rootScope_.$broadcast(
                    ERROR_EVENT_NAME, {
                        message: 'Couldn\'t download file.'
                    });
            }
        }.bind(this)
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
