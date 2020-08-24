'use strict';

goog.module('grrUi.flow.flowRequestsDirective');
goog.module.declareLegacyNamespace();



/**
 * Controller for FlowRequestsDirective.
 *
 * @constructor
 * @param {!angular.Scope} $scope
 * @ngInject
 */
const FlowRequestsController = function($scope, grrAceService, grrApiService) {
    var self = this;

    /** @private {!angular.Scope} */
    this.scope_ = $scope;

    this.grrApiService_ = grrApiService;

    this.serializedRequests = "";
    this.scope_.$watch('flowId', this.onFlowIdPathChange_.bind(this));
    this.scope_.aceConfig = function(ace) {
        self.ace = ace;

        grrAceService.AceConfig(ace);

        ace.setOptions({
            wrap: true,
            autoScrollEditorIntoView: true,
            minLines: 10,
            maxLines: 100,
        });

        self.scope_.$on('$destroy', function() {
            grrAceService.SaveAceConfig(ace);
        });

        ace.resize();
    };
};


FlowRequestsController.prototype.showSettings = function() {
    this.ace.execCommand("showSettingsMenu");
};


/**
 * Handles directive's arguments changes.
 *
 * @param {Array<string>} newValues
 * @private
 */
FlowRequestsController.prototype.onFlowIdPathChange_ = function() {
    var self = this;
    var requestsUrl = "v1/GetFlowRequests";
    var requestsParams = {
        flow_id: this.scope_['flowId'],
        client_id: this.scope_['clientId'],
    };

    this.grrApiService_.get(requestsUrl, requestsParams).then(
        function success(response) {
            self.serializedRequests = JSON.stringify(
                response.data.items, null, 2);
        });
};


/**
 * Directive for displaying requests of a flow with a given URN.
 *
 * @return {angular.Directive} Directive definition object.
 * @ngInject
 * @export
 */
exports.FlowRequestsDirective = function() {
  return {
    scope: {
      flowId: '=',
      clientId: '=',
    },
    restrict: 'E',
    templateUrl: window.base_path+'/static/angular-components/flow/flow-requests.html',
    controller: FlowRequestsController,
    controllerAs: 'controller'
  };
};

/**
 * Directive's name in Angular.
 *
 * @const
 * @export
 */
exports.FlowRequestsDirective.directive_name = 'grrFlowRequests';
