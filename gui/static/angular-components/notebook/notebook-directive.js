'use strict';

goog.module('grrUi.notebook.notebookDirective');
goog.module.declareLegacyNamespace();



const NotebookController = function($scope, grrRoutingService) {
    this.scope_ = $scope;
    this.grrRoutingService_ = grrRoutingService;
    this.state = {};

    this.grrRoutingService_.uiOnParamsChanged(
        this.scope_, ['notebookId'],
        this.onRoutingParamsChange_.bind(this));
};


NotebookController.prototype.onRoutingParamsChange_ = function(
    unused_newValues, opt_stateParams) {
    this.notebookId = opt_stateParams['notebookId'] || this.scope_['notebookId'];
    this.selectedNotebookId = opt_stateParams['notebookId'];
    this.state["notebookId"] = this.notebookId;
};

exports.NotebookDirective = function() {
  return {
    scope: {
      'notebookId': '='
    },
    restrict: 'E',
    templateUrl: window.base_path+'/static/angular-components/notebook/notebook.html',
    controller: NotebookController,
    controllerAs: 'controller'
  };
};

exports.NotebookDirective.directive_name = 'grrNotebook';
