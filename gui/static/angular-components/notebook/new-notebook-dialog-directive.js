'use strict';

goog.module('grrUi.notebook.notebookNewNotebookDialog');
goog.module.declareLegacyNamespace();


const NewNotebookDialogController = function($scope, grrApiService) {
    this.scope_ = $scope;
    this.name = "";
    this.description = "";

    this.grrApiService_ = grrApiService;
};

NewNotebookDialogController.prototype.addNotebook = function() {
    this.request = {
        name: this.name,
        description: this.description,
    };

    this.grrApiService_.post(
        'v1/NewNotebook',
        this.request).then(function success(response) {
            var onResolve = this.scope_['onResolve'];
            if (angular.isDefined(onResolve)) {
                onResolve();
            }
        }.bind(this), function failure(response) {
            var onReject = this.scope_['onReject'];
            if (angular.isDefined(onReject)) {
                onReject();
            }
        }.bind(this));
};


exports.NotebookNewNotebookDialog = function() {
    return {
        scope: {
          selectedNotebookId: '=?',
            onResolve: '&',
            onReject: '&'
        },
        restrict: 'E',
        templateUrl: '/static/angular-components/notebook/new-notebook-dialog.html',
        controller: NewNotebookDialogController,
    controllerAs: 'controller'
    };
};

exports.NotebookNewNotebookDialog.directive_name = 'grrNewNotebookDialog';
