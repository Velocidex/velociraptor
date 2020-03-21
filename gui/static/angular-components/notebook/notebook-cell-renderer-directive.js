'use strict';

goog.module('grrUi.notebook.notebookCellRendererDirective');
goog.module.declareLegacyNamespace();



const NotebookCellRendererController = function(
    $scope, grrRoutingService, grrApiService, $uibModal) {
    this.scope_ = $scope;
    this.grrRoutingService_ = grrRoutingService;
    this.grrApiService_ = grrApiService;
    this.uibModal_ = $uibModal;
    this.cell = {};
    this.id = 0;
    this.currently_editing = false;

    var self = this;
    this.scope_.$watchGroup(['cellId', 'timestamp'],
                            this.onCellIdChange_.bind(this));

    // Editing a cell only happens when the cell is in editing mode
    // and it is selected. Eventually we need to check that another
    // user does not have the cell open for editing.
    this.scope_.$watchGroup(['selected', 'controller.cell.currently_editing'],
                            function(oldvalues, newvalues) {
                                var is_editing = self.cell.currently_editing;
                                var state = self.scope_["state"];
                                var is_selected = state.selectedNotebookCellId == self.cell.cell_id;
                                self.currently_editing = is_editing && is_selected;
                            });

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
            self.id = self.cell.cell_id.split(".")[1];
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
    var cell_metadata = state.notebook.cell_metadata;

    var new_cells = [];
    for (var i=0; i<cell_metadata.length; i++) {
        if (cell_metadata[i].cell_id == cell_id && new_cells.length > 0) {
            var last_cell = new_cells.pop();
            new_cells.push(cell_metadata[i]);
            new_cells.push(last_cell);
            changed = true;
        } else {
            new_cells.push(cell_metadata[i]);
        }
    }

    if (changed) {
    state.notebook.cell_metadata = new_cells;

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
    var cell_metadata = state.notebook.cell_metadata;

    // Dont allow us to remove all cells.
    if (cell_metadata.length <= 1) {
        return;
    }

    var new_cells = [];
    for (var i=0; i<cell_metadata.length; i++) {
        if (cell_metadata[i].cell_id == cell_id) {
            changed = true;
        } else {
            new_cells.push(cell_metadata[i]);
        }
    }

    if (changed) {
    state.notebook.cell_metadata = new_cells;

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
    var cell_metadata = state.notebook.cell_metadata;

    var new_cells = [];
    for (var i=0; i<cell_metadata.length; i++) {
        if (cell_metadata[i].cell_id == cell_id && cell_metadata.length > i) {
            var next_cell = cell_metadata[i+1];
            new_cells.push(next_cell);
            new_cells.push(cell_metadata[i]);
            i += 1;
            changed = true;
        } else {
            new_cells.push(cell_metadata[i]);
        }
    }

    if (changed) {
        state.notebook.cell_metadata = new_cells;

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

NotebookCellRendererController.prototype.huntState = function(item) {
    if (item.stats.stopped) {
        return "STOPPED";
    }

    return item.state;
};


NotebookCellRendererController.prototype.huntSelect = function(event, item) {
    this.scope_["selected_hunt"] = item.hunt_id;
    event.stopPropagation();
    event.preventDefault();
    return false;
};

NotebookCellRendererController.prototype.addCellFromHunt = function(event) {
    event.stopPropagation();
    event.preventDefault();

    var self = this;
    var modalScope = this.scope_.$new();

    var modalInstance = this.uibModal_.open({
        templateUrl: '/static/angular-components/notebook/new_cell_from_hunt_modal.html',
        scope: modalScope,
        windowClass: 'wide-modal high-modal',
        size: 'lg'
    });

    modalScope.resolve = function() {
        var state = self.scope_["state"];
        var hunt_id = self.scope_["selected_hunt"];
        var request = {notebook_id: state.notebook.notebook_id,
                       type: "VQL",
                       input: "SELECT * \nFROM hunt_results(hunt_id='" + hunt_id + "')\nLIMIT 10\n",
                       currently_editing: true,
                       cell_id: self.scope_["cellId"]};

        self.grrApiService_.post(
            'v1/NewNotebookCell', request).then(function success(response) {
                state.notebook = response.data;
                state.selectedNotebookCellId = response.data['latest_cell_id'];

                modalInstance.close();

            }, function failure(response) {
                console.log("Error " + response);
            });
    };

    modalScope.reject = function() {
        modalInstance.dismiss();
    };

    self.scope_.$on('$destroy', function() {
        modalScope.$destroy();
    });


    return false;
};


NotebookCellRendererController.prototype.addCell = function(event, cell_type) {
    event.stopPropagation();
    event.preventDefault();

    var self = this;
    var state = self.scope_["state"];

    if (!cell_type) {
        cell_type = "Markdown";
    }

    var request = {notebook_id: state.notebook.notebook_id,
                   type: cell_type,
                   currently_editing: true,
                   cell_id: this.scope_["cellId"]};

    this.grrApiService_.post(
        'v1/NewNotebookCell', request).then(function success(response) {
            state.notebook = response.data;
            state.selectedNotebookCellId = response.data['latest_cell_id'];

         }, function failure(response) {
             console.log("Error " + response);
         });

    return false;
};

NotebookCellRendererController.prototype.setEditing = function(event, value) {
    this.cell.currently_editing = value;
    event.stopPropagation();
    return false;
};

NotebookCellRendererController.prototype.saveCell = function(event) {
    var url = 'v1/UpdateNotebookCell';
    var params = {notebook_id: this.scope_["notebookId"],
                  cell_id: this.scope_["cellId"],
                  type: this.cell.type || "Markdown",
                  currently_editing: false,
                  input: this.cell.input};
    var self = this;
    self.cell.output = "Loading";
    self.cell.timestamp = 0;

    this.grrApiService_.post(url, params).then(function(response) {
        self.cell = response.data;
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
            "timestamp": '=',
        },
        restrict: 'E',
        templateUrl: '/static/angular-components/notebook/notebook-cell-renderer.html',
        controller: NotebookCellRendererController,
        controllerAs: 'controller',
    };
    return result;
};

exports.NotebookCellRendererDirective.directive_name = 'grrNotebookCellRenderer';
