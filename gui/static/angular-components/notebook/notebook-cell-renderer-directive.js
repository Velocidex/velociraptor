'use strict';

goog.module('grrUi.notebook.notebookCellRendererDirective');
goog.module.declareLegacyNamespace();



const NotebookCellRendererController = function(
    $scope, grrRoutingService, grrApiService) {
    this.scope_ = $scope;
    this.grrRoutingService_ = grrRoutingService;
    this.grrApiService_ = grrApiService;
    this.cell = {};
    this.editing = false;
    this.scope_.$watch('cellId',
                       this.onCellIdChange_.bind(this));
    this.scope_.$watch('state.selectedNotebookCellId', function() {
        var state = this.scope_["state"];
        if (state.selectedNotebookCellId != this.cell.cell_id) {
            this.editing = false;
        };
    }.bind(this));

    var self = this;
    this.scope_.aceConfig = function(ace) {
        ace.commands.addCommand({
            name: 'saveAndExit',
            bindKey: {win: 'Ctrl-Enter',  mac: 'Command-Enter'},
            exec: function(editor) {
                self.saveCell();
            },
        });
    };
};

NotebookCellRendererController.prototype.onCellIdChange_ = function() {
    var request = {notebook_id: this.scope_["notebookId"],
                   cell_id: this.scope_["cellId"]};
    var self = this;
    this.grrApiService_.get(
        'v1/GetNotebookCell', request).then(function success(response) {
            self.cell = response.data;
            if (!self.cell.type) {
                self.cell.type = "Markdown";
            }
         }, function failure(response) {
             console.log("Error " + response.data);
         });
};


NotebookCellRendererController.prototype.aceConfig = function(ace) {
    var self = this;

};

NotebookCellRendererController.prototype.ace_type = function() {
    var type = this.cell.type;

    if (type == "VQL") {
        return "sql";
    }
    if (type == "Markdown") {
        return "markdown";
    }
    return "yaml";
}

NotebookCellRendererController.prototype.noop = function(e) {
    e.stopPropagation();
    return false;
};

NotebookCellRendererController.prototype.removeFocus = function(event) {
    var self = this;
    var state = self.scope_["state"];
    state.selectedNotebookCellId = null;

    event.stopPropagation();
    return false;
};

NotebookCellRendererController.prototype.upCell = function(event) {
    var self = this;
    var state = self.scope_["state"];
    var cell_id = self.scope_["cellId"];
    var changed = false;
    var cells = state.notebook.cells;

    var new_cells = [];
    for (var i=0; i<cells.length; i++) {
        if (cells[i] == cell_id && new_cells.length > 0) {
            var last_cell = new_cells.pop();
            new_cells.push(cells[i]);
            new_cells.push(last_cell);
            changed = true;
        } else {
            new_cells.push(cells[i]);
        }
    }

    if (changed) {
    state.notebook.cells = new_cells;

    this.grrApiService_.post(
        'v1/UpdateNotebook', state.notebook).then(function success(response) {
            state.notebook = response.data;

         }, function failure(response) {
             console.log("Error " + response.data);
         });
    }

    event.stopPropagation();
    return false;
};


NotebookCellRendererController.prototype.deleteCell = function(event) {
    var self = this;
    var state = self.scope_["state"];
    var cell_id = self.scope_["cellId"];
    var changed = false;
    var cells = state.notebook.cells;

    // Dont allow us to remove all cells.
    if (cells.length <= 1) {
        return;
    }

    var new_cells = [];
    for (var i=0; i<cells.length; i++) {
        if (cells[i] == cell_id) {
            changed = true;
        } else {
            new_cells.push(cells[i]);
        }
    }

    if (changed) {
    state.notebook.cells = new_cells;

    this.grrApiService_.post(
        'v1/UpdateNotebook', state.notebook).then(function success(response) {
            state.notebook = response.data;

         }, function failure(response) {
             console.log("Error " + response.data);
         });
    }

    event.stopPropagation();
    return false;
};

NotebookCellRendererController.prototype.downCell = function(event) {
    var self = this;
    var state = self.scope_["state"];
    var cell_id = self.scope_["cellId"];
    var changed = false;
    var cells = state.notebook.cells;

    var new_cells = [];
    for (var i=0; i<cells.length; i++) {
        if (cells[i] == cell_id && cells.length > i) {
            var next_cell = cells[i+1];
            new_cells.push(next_cell);
            new_cells.push(cells[i]);
            i += 1;
            changed = true;
        } else {
            new_cells.push(cells[i]);
        }
    }

    if (changed) {
        state.notebook.cells = new_cells;

        this.grrApiService_.post(
            'v1/UpdateNotebook', state.notebook).then(function success(response) {
                state.notebook = response.data;

            }, function failure(response) {
                console.log("Error " + response.data);
            });
    }

    event.stopPropagation();
    return false;
};


NotebookCellRendererController.prototype.addCell = function(event) {
    var self = this;

    var state = self.scope_["state"];
    var request = {notebook_id: state.notebook.notebook_id,
                   cell_id: this.scope_["cellId"]};

    this.grrApiService_.post(
        'v1/NewNotebookCell', request).then(function success(response) {
            state.notebook = response.data;
            state.selectedNotebookCellId = response.data['latest_cell_id'];

         }, function failure(response) {
             console.log("Error " + response);
         });

    event.stopPropagation();
    return false;
};

NotebookCellRendererController.prototype.setEditing = function(event, value) {
    this.editing = value;
    event.stopPropagation();
    return false;
};

NotebookCellRendererController.prototype.saveCell = function(event) {
    var url = 'v1/UpdateNotebookCell';
    var params = {notebook_id: this.scope_["notebookId"],
                  cell_id: this.scope_["cellId"],
                  type: this.cell.type || "Markdown",
                  input: this.cell.input};
    var self = this;
    self.cell.output = "Loading";
    self.cell.timestamp = 0;

    this.grrApiService_.post(url, params).then(function(response) {
        self.cell = response.data;

        // Update the cell on the server
        self.editing = false;
        self.onCellIdChange_();

    }, function(error) {
        self.error = error;
    });

    if (angular.isDefined(event)) {
        event.stopPropagation();
    }
    return false;
};


exports.NotebookCellRendererDirective = function() {
    var result = {
        scope: {
            'notebookId': '=',
            "cellId": '=',
            "selected": '=',
            "state": '=',
        },
        restrict: 'E',
        templateUrl: '/static/angular-components/notebook/notebook-cell-renderer.html',
        controller: NotebookCellRendererController,
        controllerAs: 'controller',
    };
    return result;
};

exports.NotebookCellRendererDirective.directive_name = 'grrNotebookCellRenderer';
