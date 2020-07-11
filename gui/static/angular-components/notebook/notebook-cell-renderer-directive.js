'use strict';

goog.module('grrUi.notebook.notebookCellRendererDirective');
goog.module.declareLegacyNamespace();



const NotebookCellRendererController = function(
    $scope, grrRoutingService, grrApiService, grrAceService, $uibModal, $timeout) {
    this.scope_ = $scope;
    this.timeout_ = $timeout;
    this.grrRoutingService_ = grrRoutingService;
    this.grrApiService_ = grrApiService;
    this.grrAceService_ = grrAceService;
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

    this.completions = [];

    this.scope_.aceConfig = function(ace) {
        self.ace = ace;
        grrAceService.AceConfig(ace);

        ace.setOptions({
            autoScrollEditorIntoView: true,
            maxLines: 25
        });

        self.ace = ace;
        self.ace.commands.addCommand({
            name: 'saveAndExit',
            bindKey: {win: 'Ctrl-Enter',  mac: 'Command-Enter'},
            exec: function(editor) {
                self.saveCell();
            },
        });
    };

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

NotebookCellRendererController.prototype.showSettings = function() {
    this.ace.execCommand("showSettingsMenu");
};

NotebookCellRendererController.prototype.onCellIdChange_ = function() {
    var request = {notebook_id: this.scope_["notebookId"],
                   cell_id: this.scope_["cellId"]};
    var self = this;
    this.grrApiService_.get(
        'v1/GetNotebookCell', request).then(function success(response) {
            // Do not close the editor if it is open.
            if (self.cell.currently_editing) {
                response.data.currently_editing = true;
            };

            self.cell = response.data;
            if (!self.cell.type) {
                self.cell.type = "Markdown";
            }
            self.id = self.cell.cell_id.split(".")[1];
         }, function failure(response) {
             console.log("Error " + response.data);
         });
};


NotebookCellRendererController.prototype.ace_type = function() {
    var type = this.cell.type;

    if (type == "VQL") {
        return "sql";
    }
    if (type == "Markdown") {
        return "markdown";
    }
    if (type == "Artifact") {
        return "yaml";
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

NotebookCellRendererController.prototype.pasteEvent = function(e) {
    var items = e.originalEvent.clipboardData.items;
    var self = this;
    var state = self.scope_["state"];

    for (var i = 0; i < items.length; i++) {
        var item = items[i];

        if (item.kind === 'file') {
            var blob = item.getAsFile();
            var reader = new FileReader();
            reader.onload = function(event) {
                var request = {
                    data: reader.result.split(",")[1],
                    notebook_id: state.notebook.notebook_id,
                    filename: blob.name,
                    size: blob.size,
                };

                self.grrApiService_.post(
                    'v1/UploadNotebookAttachment', request
                ).then(function success(response) {
                    self.timeout_(function () {
                        self.ace.insert("\n!["+blob.name+"]("+response.data.url+")\n");
                    });
                }, function failure(response) {
                    console.log("Error " + response.data);
                });
            };
            reader.readAsDataURL(blob);
        }
    }

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
    this.scope_["selected_hunt"] = item;
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
        var hunt = self.scope_["selected_hunt"];
        var hunt_id = hunt["hunt_id"];
        var query = "SELECT * \nFROM hunt_results(\n";
        var sources = hunt["artifact_sources"] || hunt["start_request"]["artifacts"];
        query += "    artifact='" + sources[0] + "',\n";
        for (var i=1; i<sources.length; i++) {
            query += "    // artifact='" + sources[i] + "',\n";
        }
        query += "    hunt_id='" + hunt_id + "')\nLIMIT 50\n";
        var request = {notebook_id: state.notebook.notebook_id,
                       type: "VQL",
                       input: query,
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


NotebookCellRendererController.prototype.addCellFromArtifact = function(event) {
    event.stopPropagation();
    event.preventDefault();

    var self = this;
    var modalScope = this.scope_.$new();
    modalScope["names"] = [];
    modalScope["params"] = {};
    modalScope["type"] = "SERVER";

    var modalInstance = this.uibModal_.open({
        templateUrl: '/static/angular-components/notebook/new_cell_from_artifact_modal.html',
        scope: modalScope,
        windowClass: 'wide-modal high-modal',
        size: 'lg'
    });

    modalScope.resolve = function() {
        var state = self.scope_["state"];
        var name = modalScope["names"][0];
        var params = modalScope["params"];
        var query = "SELECT * \nFROM Artifact." + name + "(\n";

        var lines = [];
        for (let [key, value] of Object.entries(params)) {
            if (value == '') {
                continue;
            }
            value = value.replace("'", "\\'");
            value = value.replace("\\", "\\\\");
            lines.push("   " + key + "='" + value + "'");
        }
        query += lines.join(",\n") + ")\nLIMIT 50\n";
        var request = {notebook_id: state.notebook.notebook_id,
                       type: "VQL",
                       input: query,
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


NotebookCellRendererController.prototype.addCellFromFlow = function(event) {
    event.stopPropagation();
    event.preventDefault();

    var self = this;
    var modalScope = this.scope_.$new();
    modalScope["state"] = {};

    var modalInstance = this.uibModal_.open({
        template: '<grr-new-cell-from-flow on-resolve="resolve()" '+
            'state="state"></grr-new-cell-from-flow>',
        scope: modalScope,
        windowClass: 'wide-modal high-modal',
        size: 'lg'
    });

    modalScope.resolve = function() {
        var state = self.scope_["state"];
        var flow = modalScope["state"]["flow"];
        var client_id = flow["client_id"];
        var flow_id = flow["session_id"];
        var query = "SELECT * \nFROM source(\n";
        var sources = flow["artifacts_with_results"] || flow["request"]["artifacts"];
        query += "    artifact='" + sources[0] + "',\n";
        for (var i=1; i<sources.length; i++) {
            query += "    -- artifact='" + sources[i] + "',\n";
        }
        query += "    client_id='" + client_id + "', flow_id='" +
            flow_id + "')\nLIMIT 50\n";
        var request = {notebook_id: state.notebook.notebook_id,
                       type: "VQL",
                       input: query,
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

    // Only save state when we close the editor.
    if (value == false) {
        this.grrAceService_.SaveAceConfig(this.ace);
    }

    event.stopPropagation();
    return false;
};

NotebookCellRendererController.prototype.stopCalculating = function(event) {
    var url = 'v1/CancelNotebookCell';
    var params = {notebook_id: this.scope_["notebookId"],
                  cell_id: this.scope_["cellId"],
                  type: this.cell.type || "Markdown",
                  currently_editing: false,
                  cancel: true,
                  input: this.cell.input};
    var self = this;

    this.cell.output = "Cancelling...";

    this.grrApiService_.post(url, params).then(function(response) {
        // Refresh the cell in a tick.
        self.$timeout(self.onCellIdChange_, 1000);

    }, function(error) {
        self.error = error;
    });

    if (angular.isDefined(event)) {
        event.stopPropagation();
    }
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

    self.grrAceService_.SaveAceConfig(self.ace);
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
