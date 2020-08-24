'use strict';

goog.module('grrUi.notebook.notebookNewNotebookDialog');
goog.module.declareLegacyNamespace();


const NewNotebookDialogController = function($scope, grrApiService) {
    var self = this;

    self.scope_ = $scope;
    self.notebook = self.scope_["notebook"] ||
        {name: "", description: "", collaborators:[]};
    self.users = [];
    self.collaborators = [];
    if (!angular.isArray(self.notebook.collaborators)) {
        self.notebook.collaborators = [];
    };

    for(var i=0; i<self.notebook.collaborators.length; i++) {
        self.collaborators.push({
            id: i,
            name: self.notebook.collaborators[i],
        });
    }

    self.grrApiService_ = grrApiService;
    self.username = "";
    self.grrApiService_.getCached('v1/GetUserUITraits').then(function(response) {
        self.username = response.data['username'];

        self.grrApiService_.get("v1/GetUsers").then(function(response) {
            self.user = [];
            for(var i = 0; i<response.data.users.length; i++) {
                var name = response.data.users[i].name;

                // Only add other users than the currently logged in
                // user.
                if (name != self.username) {
                    self.users.push({id: i, name: name});
                };
            }
        });
    });
};

NewNotebookDialogController.prototype.addNotebook = function() {
    var url = "v1/UpdateNotebook";

    // If this.notebook exists, then we are modifying a notebook,
    // otherwise we are making a new one.
    if (!this.notebook["notebook_id"]) {
        url = "v1/NewNotebook";
    };

    // Sync the controller list with the notebook.
    this.notebook.collaborators = [];
    for(var i=0; i<this.collaborators.length; i++) {
        this.notebook.collaborators.push(this.collaborators[i].name);
    }

    this.grrApiService_.post(url, this.notebook).then(
        function success(response) {
            var onResolve = this.scope_['onResolve'];
            if (angular.isDefined(onResolve)) {
                onResolve();
            }}.bind(this),

        function failure(response) {
            var onReject = this.scope_['onReject'];
            if (angular.isDefined(onReject)) {
                onReject();
            }
        }.bind(this));
};

exports.NotebookNewNotebookDialog = function() {
    return {
        scope: {
            notebook: '=',
            onResolve: '&',
            onReject: '&'
        },
        restrict: 'E',
        templateUrl: window.base_path+'/static/angular-components/notebook/new-notebook-dialog.html',
        controller: NewNotebookDialogController,
    controllerAs: 'controller'
    };
};

exports.NotebookNewNotebookDialog.directive_name = 'grrNewNotebookDialog';
