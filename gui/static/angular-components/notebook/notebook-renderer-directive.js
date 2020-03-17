'use strict';

goog.module('grrUi.notebook.notebookRendererDirective');
goog.module.declareLegacyNamespace();



const NotebookRendererController = function($scope, grrRoutingService, grrApiService) {
    this.scope_ = $scope;
    this.grrRoutingService_ = grrRoutingService;
    this.grrApiService_ = grrApiService;
    this.grrRoutingService_.uiOnParamsChanged(
        this.scope_, ['notebookId'],
        this.onRoutingParamsChange_.bind(this));
};


NotebookRendererController.prototype.onRoutingParamsChange_ = function(
    unused_newValues, opt_stateParams) {
    this.notebookId = opt_stateParams['notebookId'] || this.scope_['notebookId'] ;
};

NotebookRendererController.prototype.newCell = function() {
    var self = this;

    var state = self.scope_["state"];
    var request = {notebook_id: state.notebook.notebook_id};

    this.grrApiService_.post(
        'v1/NewNotebookCell', request).then(function success(response) {
            state.notebook = response.data;
            state.selectedNotebookCellId = response.data['latest_cell_id'];
         }, function failure(response) {
             console.log("Error " + response);
         });
};


NotebookRendererController.prototype.toggleFocus = function(cell_id) {
    var self = this;
    var state = self.scope_["state"] || {};

    if (state.selectedNotebookCellId != cell_id) {
        state.selectedNotebookCellId = cell_id;
    } else {
//        this.selectedNotebookCellId = null;
    }
};

exports.NotebookRendererDirective = function() {
  return {
    scope: {
        'notebookId': '=',
        'state': '=',
    },
    restrict: 'E',
    templateUrl: '/static/angular-components/notebook/notebook-renderer.html',
    controller: NotebookRendererController,
    controllerAs: 'controller'
  };
};

exports.NotebookRendererDirective.directive_name = 'grrNotebookRenderer';
