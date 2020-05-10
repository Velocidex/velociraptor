'use strict';

goog.module('grrUi.core.csvViewerDirective');
goog.module.declareLegacyNamespace();


// Angular is too slow to work with more rows.
var MAX_ROWS_PER_TABLE = 500;


/**
 * Controller for CSVViewerDirective.
 *
 * @constructor
 * @param {!angular.Scope} $scope
 * @param {!grrUi.core.apiService.ApiService} grrApiService
 * @ngInject
 */
const CsvViewerDirective = function(
    $scope, grrApiService, $uibModal, DTOptionsBuilder, DTColumnBuilder) {

    /** @private {!angular.Scope} */
    this.scope_ = $scope;

    this.uibModal_ = $uibModal;

    this.DTOptionsBuilder = DTOptionsBuilder;
    this.DTColumnBuilder = DTColumnBuilder;

    /** @private {!grrUi.core.apiService.ApiService} */
    this.grrApiService_ = grrApiService;

    /** @type {string} */
    this.baseUrl;

    /** @type {object} */
    this.params;

    /** @type {?string} */
    this.pageData;

    /** @type {object} */
    var buttons = [{
        extend: 'csv',
        className: "btn btn-default pull-left  btn-sm",
        text: '<i class="fa fa-floppy-o"></i>',
        filename: "Velociraptor Table",
        exportOptions: {
            modifier: {
                search: 'none'
            }
        }
    }];

    if (angular.isString(this.scope_["vqlHelpPlugin"])) {
        buttons.push({
            className: "btn btn-default pull-left btn-sm",
            text: '<i class="fa fa-question-circle"></i>',
            action: this.showVQL_.bind(this),
        });
    }

    this.dtOptions =  DTOptionsBuilder.newOptions()
        .withColReorder()
        .withDOM('BRlfrtip')
        .withPaginationType('full_numbers')
        .withButtons(buttons);

    this.scope_.$watch(
        'params',
        this.onContextChange_.bind(this), true);

    /** @type {object} */
    this.dtInstance = {};
};


const regex = /[^a-zA-Z0-9]/;

CsvViewerDirective.prototype.showVQL_ = function() {
    var modalScope = this.scope_.$new();

    var columns = [];
    for(var i=0;i<this.pageData["columns"].length;i++) {
        var column = this.pageData["columns"][i];
        if (regex.test(column)) {
            column = "`" + column + "`";
        }
        columns.push(column);
    }

    modalScope["vql"] = "SELECT " + columns.join(", \n    ") +
        "\nFROM " + this.scope_["vqlHelpPlugin"] + "\nLIMIT " +
        MAX_ROWS_PER_TABLE;
    modalScope["resolve"] = function(){
        modalInstance.close;
    };

    var modalInstance = this.uibModal_.open({
        template: '<grr-vql-help vql="vql"'+
            'on-resolve="resolve()" />',
        scope: modalScope,
        windowClass: 'wide-modal high-modal',
        size: 'lg'
    });
};

CsvViewerDirective.prototype.onContextChange_ = function(newValues, oldValues) {
    if (newValues != oldValues || this.pageData == null) {
        this.fetchText_();
    }
};

/**
 * Fetches the file content.
 *
 * @private
 */
CsvViewerDirective.prototype.fetchText_ = function() {
    var self = this;

    self.pageData = null;
    if (angular.isDefined(this.dtInstance.DataTable)) {
        self.dtInstance.DataTable.ngDestroy();
        var i, ths = document.querySelectorAll('#dtable th');
        for (i=0;i<ths.length;i++) {
            ths[i].removeAttribute('style');
        }
    }

    if (self.scope_.baseUrl && angular.isDefined(self.scope_.params)) {
        var url = self.scope_.baseUrl;
        var params = Object.assign({}, self.scope_.params);

        if (angular.isObject(params)) {
            params['start_row'] = 0;
            params['rows'] = MAX_ROWS_PER_TABLE;
            self.pageData = null;
            this.grrApiService_.get(url, params).then(function(response) {
              self.pageData = this.prepareData(response.data);
            }.bind(this), function() {
                self.pageData = null;
            }.bind(this)).catch(function() {
                self.pageData = null;
            });
        }
    } else if (angular.isObject(self.scope_.value)) {

        // If value is specified we expect it to be a VQLResponse so
        // we need to convert it into the same format as a csv file.
        var value = self.scope_.value;
        var rows = JSON.parse(value.Response);
        var new_rows = [];
        if (angular.isDefined(self.scope_.params)) {
            var filename = self.scope_.params.filename;
            if (filename) {
                this.dtOptions.buttons[0].filename = filename;
            }
        }

        for (var i=0; i<rows.length; i++) {
            var new_row = [];
            for (var c=0; c<value.Columns.length;c++) {
                var column = value.Columns[c];
                new_row.push(rows[i][column]);
            }

            new_rows.push({"cell": new_row});
        }

        self.pageData = {
            "columns": value.Columns,
            "rows": new_rows,
        };
    }
};

CsvViewerDirective.prototype.isObject = function(value) {
  return angular.isObject(value);
};

CsvViewerDirective.prototype.prepareData = function(value) {
  var rows = [];
  for (var i=0; i<value.rows.length; i++) {
    var row = value.rows[i].cell;
    var cells = [];
    for (var j=0; j<row.length; j++) {
      var cell = row[j];

      // A bit of a hack for now, this represents an object.
      if (cell[0] == "{" || cell[0] == "[") {
        cell = JSON.parse(cell);
      }

      cells.push(cell);
    }
    rows.push({cell: cells});
  }

  return {columns: value.columns, rows: rows};
};

/**
 * CsvViewerDirective definition.
 * @return {angular.Directive} Directive definition object.
 */
exports.CsvViewerDirective = function() {
  return {
      scope: {
          baseUrl: '=',
          params: '=',
          value: '=',
          vqlHelpPlugin: '@',
      },
      restrict: 'E',
      templateUrl: '/static/angular-components/core/csv-viewer.html',
      controller: CsvViewerDirective,
      controllerAs: 'controller',
  };
};


/**
 * Name of the directive in Angular.
 *
 * @const
 * @export
 */
exports.CsvViewerDirective.directive_name = 'grrCsvViewer';
