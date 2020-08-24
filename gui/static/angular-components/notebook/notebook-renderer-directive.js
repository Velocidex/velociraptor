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


NotebookRendererController.prototype.onRoutingParamsChange_ = function(
    unused_newValues, opt_stateParams) {
    this.notebookId = opt_stateParams['notebookId'] || this.scope_['notebookId'] ;
};

NotebookRendererController.prototype.toggleFocus = function(cell_id) {
    var self = this;
    var state = self.scope_["state"] || {};

    if (state.selectedNotebookCellId != cell_id) {
        state.selectedNotebookCellId = cell_id;
    }
};

exports.NotebookRendererDirective = function() {
  return {
    scope: {
        'notebookId': '=',
        'timestamp': '=',
        'state': '=',
    },
    restrict: 'E',
    templateUrl: window.base_path+'/static/angular-components/notebook/notebook-renderer.html',
    controller: NotebookRendererController,
    controllerAs: 'controller'
  };
};

exports.NotebookRendererDirective.directive_name = 'grrNotebookRenderer';
