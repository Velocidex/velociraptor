'use strict';

goog.module('grrUi.artifact.csvFormDirective');

const CSVFormController = function($scope) {
    this.scope_ = $scope;
    this.columns = [];
    this.rows = [];

    $scope.$watch('value[field]', this.onValueChange_.bind(this));
};


CSVFormController.prototype.removeRow = function(e, row_index) {
    this.rows.splice(row_index, 1);

    this.scope_["value"][this.scope_["field"]] = $.csv.fromArrays(
        [this.columns,].concat(this.rows));

    e.stopPropagation();
    e.preventDefault();
    return false;
};

CSVFormController.prototype.insertRow = function(e, row_index) {
    var new_row = Array(this.columns.length).fill("");
    this.rows.splice(row_index, 0, new_row);

    this.scope_["value"][this.scope_["field"]] = $.csv.fromArrays(
        [this.columns,].concat(this.rows));

    e.stopPropagation();
    e.preventDefault();
    return false;
};


CSVFormController.prototype.setEditableCellValue = function(e, item, current_row, current_column) {
    if (angular.isString(item)) {
        this.rows[current_row][current_column] = item;

        this.scope_["value"][this.scope_["field"]] = $.csv.fromArrays(
            [this.columns,].concat(this.rows));
    }

    e.stopPropagation();
    e.preventDefault();
    return false;
};


CSVFormController.prototype.onValueChange_ = function() {
    var data = this.scope_["value"][this.scope_["field"]];
    try {
        this.rows = [];
        var lines = $.csv.parsers.splitLines(data);
        for (var i=0; i<lines.length; i++) {
            if (i==0) {
                this.columns = $.csv.toArray(lines[0]);
            } else {
                this.rows.push($.csv.toArray(lines[i]));
            }
        }
    } catch(e) {
        this.rows = [];
    };
};


exports.CSVFormDirective = function() {
  return {
    restrict: 'E',
    scope: {
        field: '=',
        value: '=',
    },
    templateUrl: window.base_path+'/static/angular-components/artifact/csv-form.html',
    controller: CSVFormController,
    controllerAs: 'controller'
  };
};


exports.CSVFormDirective.directive_name = 'grrCsvForm';
