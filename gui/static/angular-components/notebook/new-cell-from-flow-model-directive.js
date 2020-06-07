'use strict';

goog.module('grrUi.notebook.newCellFromFlowModelDirective');

const NewCellFromFlowModelController = function(
    $scope, grrApiService) {
    var self = this;

    this.scope_ = $scope;
    this.grrApiService_ = grrApiService;

    this.query;
    this.clientId;
    this.flowsUrl;

    // This will hold the client table update function.
    this.triggerClientTableUpdate;
    this.triggerFlowsTableUpdate;

    this.scope_.$watch("controller.query", function() {
        if (angular.isFunction(self.triggerClientTableUpdate)) {
            self.triggerClientTableUpdate();
        };

        if (angular.isFunction(self.triggerFlowsTableUpdate)) {
            self.triggerFlowsTableUpdate();
        };
    });
};

NewCellFromFlowModelController.prototype.selectFlow = function(item) {
    var state = this.scope_["state"];
    state["flow"] = item;

    this.flowId = item["session_id"];
    var resolve = this.scope_['onResolve'];
    if (angular.isFunction(resolve)) {
        resolve();
    };
};

NewCellFromFlowModelController.prototype.onClientClick = function(item) {
    this.clientId = item["client_id"];
    this.flowsUrl = '/v1/GetClientFlows/' + this.clientId;
};

exports.NewCellFromFlowModelDirective = function() {
    var result = {
        scope: {
            state: "=",
            onResolve: "&",
            onReject: "&",
        },
        restrict: 'E',
        templateUrl: '/static/angular-components/notebook/new_cell_from_flow_modal.html',
        controller: NewCellFromFlowModelController,
        controllerAs: 'controller',
    };
    return result;
};

exports.NewCellFromFlowModelDirective.directive_name = 'grrNewCellFromFlow';
