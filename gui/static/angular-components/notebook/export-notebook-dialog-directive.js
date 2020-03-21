'use strict';

goog.module('grrUi.notebook.notebookExportNotebookDialog');
goog.module.declareLegacyNamespace();

let AUTO_REFRESH_INTERVAL_MS = 5 * 1000;

const ExportNotebookDialogController = function($scope, grrApiService) {
    var self = this;

    self.scope_ = $scope;
    self.grrApiService_ = grrApiService;
}

ExportNotebookDialogController.prototype.exportNotebook = function(event) {
    this.request = {
        notebook_id: this.scope_["notebook"]['notebook_id'],
        type: "html",
    };

    this.grrApiService_.post(
        'v1/CreateNotebookDownloadFile',
        this.request).then(function success(response) {
            return;

        }.bind(this), function failure(response) {
            return;

        }.bind(this));

    event.stopPropagation();
    event.preventDefault();
    return false;
};


exports.NotebookExportNotebookDialog = function() {
    return {
        scope: {
          notebook: '=',
            onResolve: '&',
            onReject: '&'
        },
        restrict: 'E',
        templateUrl: '/static/angular-components/notebook/export-notebook-dialog.html',
        controller: ExportNotebookDialogController,
    controllerAs: 'controller'
    };
};

exports.NotebookExportNotebookDialog.directive_name = 'grrExportNotebookDialog';
