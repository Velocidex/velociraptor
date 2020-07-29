'use strict';

goog.module('grrUi.client.shellCellDirective');

const ShellCellController = function($scope, grrApiService) {
    var self = this;

    /** @private {!angular.Scope} */
    this.scope_ = $scope;
    this.grrApiService_ = grrApiService;

    // The command ran on the endpoint.
    this.input = "";

    // The stdout and stderr we got.
    this.stdout = "";
    this.stderr = "";

    this.loaded = false;
    this.collapsed = false;

    this.flow = this.scope_["flow"];

    if (!angular.isObject(this.flow)) {
        return;
    }

    if (!angular.isArray(this.flow.request.artifacts) ||
        this.flow.request.artifacts.length == 0) {
        return;
    }

    // Figure out the command we requested.
    var parameters = this.flow.request.parameters.env;
    for(var i=0; i<parameters.length; i++) {
        if (parameters[i].key == 'Command') {
            self.input = parameters[i].value;
        };
    }

    this.artifact = this.flow.request.artifacts[0];
};

ShellCellController.prototype.loadData = function() {
    var self = this;

    this.loaded = true;

    this.grrApiService_.get("/v1/GetTable", {
        artifact: this.artifact,
        client_id: this.flow["client_id"],
        flow_id: this.flow["session_id"],
        rows: 500,
    }).then(function(response) {
        self.data = [];
        for(var row=0; row<response.data.rows.length; row++) {
            var item = {};
            var current_row = response.data.rows[row].cell;
            for(var column=0; column<response.data.columns.length; column++) {
                item[response.data.columns[column]] = current_row[column];
            }
            self.data.push(item);
        }
    });
};

exports.ShellCellDirective = function() {
  return {
    scope: {
      'flow': '='
    },
    restrict: 'E',
    templateUrl: '/static/angular-components/client/shell-cell.html',
    controller: ShellCellController,
    controllerAs: 'controller'
  };
};

exports.ShellCellDirective.directive_name = 'grrShellCell';
