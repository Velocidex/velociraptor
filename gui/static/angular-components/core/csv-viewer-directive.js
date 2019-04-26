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
    $scope, grrApiService, DTOptionsBuilder, DTColumnBuilder) {

    /** @private {!angular.Scope} */
    this.scope_ = $scope;

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
    this.options = {
        "pagingType": "full_numbers"
    };

    this.scope_.$watch(
        'params',
        this.onContextChange_.bind(this), true);

    /** @type {object} */
    this.dtInstance = {};
};

/**
 * Handles changes to the clientId and filePath.
 *
 * @private
 */
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
        var params = self.scope_.params;
        if (angular.isObject(params) && angular.isDefined(params.path)) {
            params['start_row'] = 0;
            params['rows'] = MAX_ROWS_PER_TABLE;
            self.pageData = null;
            this.grrApiService_.get(url, params).then(function(response) {
                self.pageData = response.data;
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
